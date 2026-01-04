# Polyinsider Technical Documentation

This document provides detailed technical information about data flows, application logic, and implementation details.

---

## 1. Application Startup Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         main.go Startup Sequence                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  1. Load Configuration                                                   │
│     └─► config.Load()                                                   │
│         ├─► Load .env file (godotenv)                                   │
│         ├─► Read environment variables                                  │
│         ├─► Apply defaults for missing values                           │
│         └─► Validate configuration                                      │
│                                                                          │
│  2. Initialize Logger                                                    │
│     └─► setupLogger(cfg.LogLevel)                                       │
│         └─► Configure slog with custom time format                      │
│                                                                          │
│  3. Setup Signal Handling                                                │
│     └─► Listen for SIGINT/SIGTERM                                       │
│                                                                          │
│  4. Fetch Active Markets                                                 │
│     └─► ingest.GetActiveTokenIDs(100)                                   │
│         ├─► HTTP GET to Gamma API                                       │
│         ├─► Parse market response                                       │
│         └─► Extract clobTokenIds                                        │
│                                                                          │
│  5. Start WebSocket Listener                                             │
│     └─► listener.SetAssetIDs(tokenIDs)                                  │
│     └─► listener.Start(ctx)                                             │
│         ├─► goroutine: runLoop (connect + read)                         │
│         └─► goroutine: heartbeatMonitor                                 │
│                                                                          │
│  6. Start Trade Logger                                                   │
│     └─► goroutine: logTrades(ctx, tradeChan, cfg)                       │
│                                                                          │
│  7. Wait for Shutdown Signal                                             │
│     └─► Block on sigChan                                                │
│                                                                          │
│  8. Graceful Shutdown                                                    │
│     ├─► Cancel context                                                  │
│     ├─► listener.Stop()                                                 │
│     └─► drainTrades()                                                   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 2. WebSocket Data Flow

### 2.1 Connection Lifecycle

```
┌──────────────────────────────────────────────────────────────────────────┐
│                      WebSocket Connection Flow                            │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐                 │
│  │   CONNECT   │────▶│  SUBSCRIBE  │────▶│   READING   │                 │
│  └─────────────┘     └─────────────┘     └──────┬──────┘                 │
│         ▲                                       │                         │
│         │                                       │ Error/Timeout           │
│         │         ┌─────────────┐               │                         │
│         └─────────│   BACKOFF   │◀──────────────┘                         │
│                   │  (1s→60s)   │                                         │
│                   └─────────────┘                                         │
│                                                                           │
└──────────────────────────────────────────────────────────────────────────┘

Backoff Parameters:
- Initial: 1 second
- Maximum: 60 seconds  
- Factor: 2x
- Jitter: ±20%
```

### 2.2 Message Processing Pipeline

```
┌──────────────────────────────────────────────────────────────────────────┐
│                     Message Processing Pipeline                           │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  WebSocket Frame                                                          │
│       │                                                                   │
│       ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │  conn.ReadMessage()                                                  │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│       │                                                                   │
│       ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │  handleMessage(data []byte)                                          │ │
│  │  └─► ParseMessage(data)                                              │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│       │                                                                   │
│       ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │  Parse Attempt Order:                                                │ │
│  │  1. Try []BookEvent (array of orderbook snapshots)                   │ │
│  │  2. Try single BookEvent                                             │ │
│  │  3. Try WSMessage wrapper                                            │ │
│  │  4. Try last_trade_price event                                       │ │
│  │  5. Try trade event                                                  │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│       │                                                                   │
│       ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │  Extract []store.Trade from events                                   │ │
│  │  - Generate unique ID                                                │ │
│  │  - Parse timestamp                                                   │ │
│  │  - Extract price from last_trade_price field                         │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│       │                                                                   │
│       ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │  tradeChan <- trade (buffered, 1000 capacity)                        │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                                                           │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Polymarket API Message Formats

### 3.1 Book Event (Primary Data Source)

The market channel primarily sends `book` events - orderbook snapshots:

```json
{
  "market": "0xe9c127a8c35f045d37b5344b0a36711084fa20c2fc1618bf178a5386f90610be",
  "asset_id": "80230236018433940569996058935444651347308266584430516013229815570706872276819",
  "timestamp": "1767527823560",
  "hash": "74bf642afe62d3791767cfc73794b5583f173417",
  "event_type": "book",
  "last_trade_price": "0.058",
  "bids": [
    {"price": "0.055", "size": "1000"},
    {"price": "0.054", "size": "500"}
  ],
  "asks": [
    {"price": "0.060", "size": "800"},
    {"price": "0.061", "size": "1200"}
  ]
}
```

**Key Fields:**
| Field | Type | Description |
|-------|------|-------------|
| `market` | string | Condition ID (market identifier) |
| `asset_id` | string | Token ID (YES/NO outcome token) |
| `timestamp` | string | Unix timestamp in milliseconds |
| `event_type` | string | Event type: `book`, `price_change` |
| `last_trade_price` | string | Price of most recent trade (0-1) |
| `bids` | array | Buy orders (descending by price) |
| `asks` | array | Sell orders (ascending by price) |

### 3.2 Price Change Event

```json
{
  "market": "0xe9c127a8...",
  "asset_id": "8023023601...",
  "timestamp": "1767527823560",
  "event_type": "price_change",
  "changes": [
    {"price": "0.055", "side": "BID", "delta": "100"}
  ]
}
```

### 3.3 Subscription Message

```json
{
  "type": "market",
  "assets_ids": [
    "93592949212798121127213117304912625505836768562433217537850469496310204567695",
    "3074539347152748632858978545166555332546941892131779352477699494423276162345"
  ]
}
```

---

## 4. Data Models

### 4.1 Trade Struct

```go
type Trade struct {
    ID              string    // Unique identifier (generated)
    MarketID        string    // Condition ID
    AssetID         string    // Token ID
    MakerAddress    string    // Maker wallet (empty from book events)
    TakerAddress    string    // Taker wallet (empty from book events)
    Side            string    // BUY or SELL
    Outcome         string    // YES or NO
    Size            string    // Trade size (raw string)
    Price           float64   // Execution price (0-1)
    ValueUSD        float64   // Calculated USD value
    Timestamp       time.Time // Event timestamp
    TradeID         string    // Original trade ID from Polymarket
    TransactionHash string    // On-chain tx hash (if available)
}
```

### 4.2 Config Struct

```go
type Config struct {
    // Polymarket
    PolymarketWSURL string  // WebSocket endpoint

    // RPC
    AlchemyAPIKey   string
    AlchemyURL      string
    FallbackRPCURL  string

    // Thresholds
    MinValueUSD      float64       // Minimum trade value to track
    WhaleValueUSD    float64       // Whale threshold
    FreshWalletNonce int           // Max nonce for "fresh" wallet
    BurstCount       int           // Trades in window for burst
    BurstWindow      time.Duration // Burst detection window

    // Alerting
    DiscordWebhookURL  string
    AlertBatchDuration time.Duration
    AlertCooldown      time.Duration

    // Database
    DBPath string

    // Workers
    WorkerCount int

    // Metrics
    PrometheusPort int

    // Logging
    LogLevel string
}
```

---

## 5. Goroutine Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                       Goroutine Architecture                             │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  main goroutine                                                          │
│       │                                                                  │
│       ├──► [goroutine 1] listener.runLoop()                             │
│       │         │                                                        │
│       │         └──► Reads from WebSocket                               │
│       │              Sends to tradeChan                                 │
│       │                                                                  │
│       ├──► [goroutine 2] listener.heartbeatMonitor()                    │
│       │         │                                                        │
│       │         └──► Checks lastMsg timestamp every 10s                 │
│       │              Sends ping if no activity for 60s                  │
│       │                                                                  │
│       ├──► [goroutine 3] logTrades()                                    │
│       │         │                                                        │
│       │         └──► Reads from tradeChan                               │
│       │              Logs trade info                                    │
│       │              Filters by value threshold                         │
│       │              Periodic stats every 30s                           │
│       │                                                                  │
│       └──► [main] Waits on sigChan for shutdown                         │
│                                                                          │
│  Channels:                                                               │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │  tradeChan: chan store.Trade (buffered, cap=1000)                   │ │
│  │  - Producer: WebSocket listener                                     │ │
│  │  - Consumer: logTrades (temporary), workers (future)                │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │  stopChan: chan struct{} (unbuffered, closed on shutdown)           │ │
│  │  - Signals goroutines to stop                                       │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 6. External API Integration

### 6.1 Gamma API (Market Discovery)

**Endpoint:** `https://gamma-api.polymarket.com/markets`

**Request:**
```http
GET /markets?active=true&closed=false&limit=100
```

**Response (truncated):**
```json
[
  {
    "id": "516926",
    "question": "MicroStrategy sells any Bitcoin in 2025?",
    "conditionId": "0x19ee98e348c0ccb341d1b9566fa14521566e9b2ea7aed34dc407a0ec56be36a2",
    "slug": "microstrategy-sell-any-bitcoin-in-2025",
    "active": true,
    "closed": false,
    "volume": "17818693.479867",
    "liquidity": "1194121.11438",
    "clobTokenIds": "[\"93592949212798121127213117304912625505836768562433217537850469496310204567695\", \"3074539347152748632858978545166555332546941892131779352477699494423276162345\"]",
    "outcomes": "[\"Yes\", \"No\"]",
    "outcomePrices": "[\"0.0005\", \"0.9995\"]"
  }
]
```

**Key Fields for Token Extraction:**
- `clobTokenIds`: JSON string array of token IDs (YES token, NO token)
- `active`: Must be true
- `closed`: Must be false

### 6.2 CLOB WebSocket

**Endpoint:** `wss://ws-subscriptions-clob.polymarket.com/ws/market`

**Connection Headers:**
```
Origin: https://polymarket.com
```

**Subscription:**
```json
{"type": "market", "assets_ids": ["token_id_1", "token_id_2", ...]}
```

**Response Format:** Array of BookEvent objects (see Section 3.1)

---

## 7. Configuration Reference

### 7.1 Environment Variables

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `POLYMARKET_WS_URL` | string | `wss://ws-subscriptions-clob.polymarket.com/ws/` | WebSocket base URL |
| `ALCHEMY_API_KEY` | string | *(required)* | Alchemy API key for RPC |
| `ALCHEMY_URL` | string | `https://polygon-mainnet.g.alchemy.com/v2/` | Alchemy base URL |
| `FALLBACK_RPC_URL` | string | `https://polygon-rpc.com` | Fallback RPC endpoint |
| `MIN_VALUE_USD` | float | `2000` | Minimum trade value to process |
| `WHALE_VALUE_USD` | float | `50000` | Whale detection threshold |
| `FRESH_WALLET_NONCE` | int | `5` | Max nonce for fresh wallet |
| `BURST_COUNT` | int | `3` | Trades for burst detection |
| `BURST_WINDOW_SECONDS` | int | `60` | Burst detection window |
| `DISCORD_WEBHOOK_URL` | string | *(optional)* | Discord webhook for alerts |
| `ALERT_BATCH_SECONDS` | int | `30` | Alert batching window |
| `ALERT_COOLDOWN_MINUTES` | int | `60` | Per-wallet alert cooldown |
| `DB_PATH` | string | `./data/trades.db` | SQLite database path |
| `WORKER_COUNT` | int | `5` | Number of worker goroutines |
| `PROMETHEUS_PORT` | int | `9090` | Metrics server port |
| `LOG_LEVEL` | string | `INFO` | Log level (DEBUG/INFO/WARN/ERROR) |

### 7.2 Loading Priority

1. Environment variables (highest priority)
2. `.env` file
3. Hardcoded defaults (lowest priority)

---

## 8. Error Handling

### 8.1 WebSocket Errors

| Error | Handling |
|-------|----------|
| Connection refused | Exponential backoff retry (1s → 60s) |
| 404 Not Found | Check endpoint path, retry with backoff |
| Read timeout | Force reconnect after 70s of no messages |
| Write error | Log warning, close connection |
| JSON parse error | Log at DEBUG level, skip message |

### 8.2 Channel Overflow

If `tradeChan` is full (1000 trades buffered):
- Log warning with dropped trade ID
- Continue processing (non-blocking send)

---

## 9. Logging Format

### 9.1 Standard Format

```
time="2026-01-04 14:32:01" level=INFO msg=event_name key1=value1 key2=value2
```

### 9.2 Key Events

| Event | Level | Fields |
|-------|-------|--------|
| `polyinsider_starting` | INFO | version |
| `config_loaded` | INFO | all config values (secrets masked) |
| `fetched_active_markets` | INFO | market_count, token_count |
| `ws_connected` | INFO | endpoint |
| `ws_subscribed` | INFO | channel, asset_count |
| `trade_received` | DEBUG | id, market, maker, side, size, price, value_usd |
| `high_value_trade` | INFO | (same as trade_received) |
| `trade_stats` | INFO | total_trades, filtered_trades |
| `ws_connect_failed` | ERROR | error, backoff |
| `shutdown_signal_received` | INFO | signal |
| `shutdown_complete` | INFO | - |

---

## 10. Future Enhancements

### 10.1 Real Trade Data Sources

To get actual trade events with maker/taker/size:

1. **On-chain Event Monitoring**
   - Listen for `OrderFilled` events on Polygon
   - Requires WebSocket connection to Alchemy/QuickNode
   - Higher latency but complete data

2. **REST API Polling**
   - Poll `GET /trades` endpoint periodically
   - Lower frequency but simpler implementation

3. **User Channel (Authenticated)**
   - Requires Polymarket API credentials
   - Provides real-time trade notifications for authenticated user

### 10.2 Planned Features

- [ ] Worker pool for parallel RPC enrichment
- [ ] SQLite persistence layer
- [ ] Discord alert formatting
- [ ] Prometheus metrics endpoint
- [ ] Nonce caching with TTL
- [ ] Circuit breaker for RPC failures

