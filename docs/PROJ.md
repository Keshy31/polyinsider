
# Project: Polyinsider

## 1. Executive Summary

**Objective:** Build a low-latency surveillance system to detect insider trading and anomalous whale activity on Polymarket (Polygon Network).
**Philosophy:** Speed first, Intelligence second.

* **Phase 1 (The Speed Trap):** A deterministic, high-concurrency Go engine that alerts on hard rules (High Value + Fresh Wallet) in < 500ms.
* **Phase 2 (The Minority Report):** *(Out of scope)* An asynchronous Python sidecar that analyzes clusters, graph relationships, and sentiment for deeper signal generation.

---

## 2. System Architecture

### 2.1 High-Level Data Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              POLYINSIDER                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐    ┌───────────┐  │
│  │  Polymarket  │───▶│   Listener   │───▶│   Worker     │───▶│  Writer   │  │
│  │  WebSocket   │    │  (1 routine) │    │   Pool       │    │ (1 routine│  │
│  └──────────────┘    └──────────────┘    │  (5-10)      │    └───────────┘  │
│                             │            └──────────────┘          │        │
│                             │                   │                  │        │
│                             ▼                   ▼                  ▼        │
│                      ┌─────────────┐     ┌───────────┐      ┌───────────┐   │
│                      │  In-Memory  │     │  Polygon  │      │  SQLite   │   │
│                      │   State     │     │   RPC     │      │    DB     │   │
│                      │  (TTL Map)  │     │ (Alchemy) │      └───────────┘   │
│                      └─────────────┘     └───────────┘                      │
│                             │                                               │
│                             ▼                                               │
│                      ┌─────────────┐                                        │
│                      │   Alert     │───▶  Discord Webhook                   │
│                      │  Batcher    │                                        │
│                      └─────────────┘                                        │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Core Pipeline Steps

1. **Ingestion:** Connect to Polymarket WebSocket (CLOB subscription).
2. **Filtering:** Immediate discard of low-value noise (< $2,000).
3. **State Tracking:** Update in-memory map for burst detection.
4. **Enrichment:** Concurrent RPC calls to Alchemy (Polygon) to verify wallet nonce.
5. **Persistence:** Batch write filtered "Suspects" to SQLite.
6. **Alerting:** Batch and push formatted payloads to Discord Webhook.

---

## 3. Polymarket API Integration

### 3.1 WebSocket Endpoints

| Endpoint | Purpose | Status |
|----------|---------|--------|
| `wss://ws-subscriptions-clob.polymarket.com/ws/market` | Market channel (orderbook, price changes) | **Active** ✅ |
| `wss://ws-subscriptions-clob.polymarket.com/ws/user` | User channel (authenticated trades/orders) | Requires API Key |
| `https://gamma-api.polymarket.com/markets` | REST API for active markets | **Active** ✅ |

> **✅ Implementation Complete (Jan 2026):** We verified the API schema through live testing:
> - Market channel provides `book` and `price_change` events
> - `book` events include `last_trade_price` field (price only, no size/maker)
> - Must subscribe to specific `asset_ids` (token IDs) - empty array does NOT subscribe to all
> - Token IDs fetched dynamically from Gamma API

### 3.2 Actual Message Flow (Verified)

```
┌──────────┐                              ┌──────────────┐
│  Client  │                              │  Polymarket  │
└────┬─────┘                              └──────┬───────┘
     │                                           │
     │  1. Fetch active markets (REST)           │
     │  GET /markets?active=true&limit=100       │
     │ ─────────────────────────────────────────▶│
     │                                           │
     │  2. Extract clobTokenIds from response    │
     │◀───────────────────────────────────────── │
     │                                           │
     │  3. Connect to WSS /ws/market             │
     │ ─────────────────────────────────────────▶│
     │                                           │
     │  4. Subscribe with token IDs              │
     │  {"type":"market",                        │
     │   "assets_ids":["token1","token2",...]}   │
     │ ─────────────────────────────────────────▶│
     │                                           │
     │  5. Receive orderbook events (array)      │
     │  [{"event_type":"book","market":"0x...",  │
     │    "asset_id":"...",                      │
     │    "last_trade_price":"0.65",             │
     │    "bids":[...],"asks":[...]}]            │
     │◀───────────────────────────────────────── │
     │                                           │
```

### 3.3 Subscription Strategy

**Current Implementation: Targeted Markets (Option B)**
- Fetch top 100 active markets from Gamma API at startup
- Extract `clobTokenIds` from each market (typically 2 per market: YES/NO)
- Subscribe to all extracted token IDs (~200 tokens)
- Receive real-time orderbook updates for subscribed markets

**Limitation Discovered:**
The market channel provides **orderbook data**, not individual trade events. To get actual trades with maker/taker addresses and sizes, we would need:
1. On-chain event monitoring (Polygon blockchain)
2. Authenticated user channel (requires API credentials)
3. REST API polling for recent trades

For Phase 1, we use `last_trade_price` changes as trade signals.

---

## 4. Phase 1: The MVP ("The Speed Trap")

**Goal:** Deploy a functional surveillance system that alerts on high-value trades from fresh wallets.
**Latency Target:** < 500ms from WebSocket message to Discord alert.

### 4.1 Tech Stack

| Component | Technology | Rationale |
|-----------|------------|-----------|
| Core Engine | Go (Golang) | Native concurrency (goroutines), low overhead, single binary |
| Data Source | Polymarket CLOB WebSocket | Real-time trade stream |
| Blockchain RPC | Alchemy (Polygon Mainnet) | Reliable, high rate limits on paid tier |
| Database | SQLite | Battle-tested Go support, zero server maintenance, portable |
| Notifications | Discord Webhook | Simple HTTP POST, rich embeds |
| Metrics | Prometheus | Industry standard, easy local scraping |
| Config | `.env` file | 12-factor app style, simple for local development |

### 4.2 Signal Definitions (Hard Rules)

#### Signal A: Fresh Insider 🔴 (Priority: HIGH)
```
IF value_usd > 2000 AND wallet_nonce < 5 THEN ALERT
```
- **Hypothesis:** User created a wallet specifically to bet on this event using inside info.
- **Enrichment Required:** RPC call to `eth_getTransactionCount`

#### Signal B: Whale 🐋 (Priority: MED)
```
IF value_usd > 50000 THEN ALERT
```
- **Hypothesis:** Major market mover; potential to shift odds significantly.
- **Enrichment Required:** None (value-based only)

#### Signal C: Panic Burst ⚡ (Priority: LOW)
```
IF trades_from_address_in_last_60s >= 3 THEN ALERT
```
- **Hypothesis:** User is trying to enter a position quickly before news breaks.
- **Enrichment Required:** In-memory state tracking

### 4.3 Alert Batching Strategy

To avoid Discord spam and alert fatigue:

| Scenario | Behavior |
|----------|----------|
| Single qualifying trade | Immediate alert |
| Multiple trades from same wallet in 60s | Batch into single summary alert |
| Same wallet already alerted | Cooldown: 1 hour before re-alerting |

**Batch Window:** 30 seconds  
**Flush Trigger:** End of batch window OR buffer reaches 10 alerts

### 4.4 Concurrency Model

```go
// Conceptual goroutine structure
func main() {
    tradeChan := make(chan Trade, 1000)      // Buffered channel
    alertChan := make(chan Alert, 100)       // Alert queue
    
    go listener(tradeChan)                   // 1 goroutine: WS → channel
    
    for i := 0; i < workerCount; i++ {
        go worker(tradeChan, alertChan)      // N goroutines: enrich + filter
    }
    
    go batcher(alertChan)                    // 1 goroutine: batch alerts
    go writer(tradeChan)                     // 1 goroutine: DB writes
    go metricsServer()                       // 1 goroutine: Prometheus /metrics
}
```

**Worker Pool Size:** 5-10 goroutines (configurable via env)
- Bottleneck is RPC latency (~50-200ms per call)
- More workers = higher RPC throughput but watch rate limits

### 4.5 State Management (In-Memory)

For Signal C (Panic Burst), we need a time-windowed counter per address:

```go
type BurstTracker struct {
    mu     sync.RWMutex
    trades map[string][]time.Time  // address → timestamps
    ttl    time.Duration           // 60 seconds
}

func (b *BurstTracker) Record(address string) int {
    // Add timestamp, prune old entries, return count
}
```

**Trade-offs:**
- ✅ Fast O(1) lookups
- ✅ No external dependency
- ❌ State lost on restart (acceptable for Phase 1)

---

## 5. Database Design

### 5.1 SQLite Schema

```sql
-- Core trades table
CREATE TABLE IF NOT EXISTS trades (
    id TEXT PRIMARY KEY,                    -- UUID v4
    market_id TEXT NOT NULL,
    market_name TEXT,                       -- Human readable (if available)
    asset_id TEXT NOT NULL,
    maker_address TEXT NOT NULL,
    taker_address TEXT,                     -- May be null in some trade types
    side TEXT NOT NULL,                     -- 'BUY' or 'SELL'
    outcome TEXT,                           -- 'YES' or 'NO' 
    size_raw TEXT NOT NULL,                 -- Raw value from API (string to preserve precision)
    value_usd REAL NOT NULL,                -- Calculated USD value
    price REAL,                             -- Execution price (0-1 range)
    nonce INTEGER,                          -- Wallet transaction count (null if not enriched)
    signal_type TEXT NOT NULL,              -- 'FRESH_INSIDER', 'WHALE', 'PANIC_BURST'
    created_at TEXT DEFAULT (datetime('now')),
    
    -- Indexes for common queries
    CONSTRAINT valid_signal CHECK (signal_type IN ('FRESH_INSIDER', 'WHALE', 'PANIC_BURST'))
);

CREATE INDEX idx_trades_maker ON trades(maker_address);
CREATE INDEX idx_trades_signal ON trades(signal_type);
CREATE INDEX idx_trades_created ON trades(created_at);
CREATE INDEX idx_trades_market ON trades(market_id);

-- Alerts table (for deduplication and history)
CREATE TABLE IF NOT EXISTS alerts (
    id TEXT PRIMARY KEY,
    trade_ids TEXT NOT NULL,                -- JSON array of related trade IDs
    wallet_address TEXT NOT NULL,
    signal_type TEXT NOT NULL,
    summary TEXT NOT NULL,                  -- Discord message content
    sent_at TEXT DEFAULT (datetime('now')),
    discord_success INTEGER DEFAULT 0       -- 1 if webhook succeeded
);

CREATE INDEX idx_alerts_wallet ON alerts(wallet_address);
CREATE INDEX idx_alerts_sent ON alerts(sent_at);
```

### 5.2 Write Batching

To avoid lock contention on SQLite:

```go
const (
    BatchSize     = 100   // Flush after N records
    BatchInterval = 1     // Flush after N seconds (whichever comes first)
)
```

Writer goroutine accumulates trades and flushes when either threshold is hit.

---

## 6. RPC Strategy

### 6.1 Primary: Alchemy

```
Endpoint: https://polygon-mainnet.g.alchemy.com/v2/{API_KEY}
Method:   eth_getTransactionCount
Params:   [address, "latest"]
```

### 6.2 Fallback: Public RPC

If Alchemy fails or rate-limits:

```
Endpoint: https://polygon-rpc.com
```

> ⚠️ Public RPCs are unreliable and rate-limited. Use only as fallback.

### 6.3 Rate Limit Mitigation

1. **Aggressive filtering:** Only call RPC for trades > $2,000 (reduces calls by ~95%)
2. **Caching:** Cache nonce lookups for 5 minutes (wallet nonce doesn't change frequently)
3. **Circuit breaker:** If 5 consecutive RPC failures, pause enrichment for 30 seconds

---

## 7. Configuration

### 7.1 Environment Variables (`.env`)

```bash
# === Polymarket ===
POLYMARKET_WS_URL=wss://ws-subscriptions-clob.polymarket.com/ws/

# === Blockchain RPC ===
ALCHEMY_API_KEY=your_alchemy_key_here
ALCHEMY_URL=https://polygon-mainnet.g.alchemy.com/v2/
FALLBACK_RPC_URL=https://polygon-rpc.com

# === Thresholds ===
MIN_VALUE_USD=2000
WHALE_VALUE_USD=50000
FRESH_WALLET_NONCE=5
BURST_COUNT=3
BURST_WINDOW_SECONDS=60

# === Alerting ===
DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/...
ALERT_BATCH_SECONDS=30
ALERT_COOLDOWN_MINUTES=60

# === Database ===
DB_PATH=./data/trades.db

# === Workers ===
WORKER_COUNT=5

# === Metrics ===
PROMETHEUS_PORT=9090
```

### 7.2 Configuration Loading Priority

1. Environment variables (highest priority)
2. `.env` file
3. Hardcoded defaults (lowest priority)

---

## 8. Observability

### 8.1 Logging

**Format:** Structured text logs with timestamp and level

```
2025-01-04 14:32:01 [INFO]  ws_connected endpoint=wss://ws-subscriptions-clob.polymarket.com/ws/
2025-01-04 14:32:02 [DEBUG] trade_received market=0x123 value=5000.00
2025-01-04 14:32:02 [INFO]  signal_detected type=FRESH_INSIDER wallet=0xabc value=5000.00 nonce=2
2025-01-04 14:32:03 [WARN]  rpc_slow latency_ms=450 endpoint=alchemy
2025-01-04 14:32:33 [INFO]  alert_sent webhook=discord trades=3
```

**Log Levels:**
- `DEBUG`: All trade events, state changes
- `INFO`: Signals detected, alerts sent, connections
- `WARN`: Slow RPCs, reconnection attempts
- `ERROR`: Failed webhooks, RPC errors, DB errors

### 8.2 Prometheus Metrics

Exposed on `http://localhost:9090/metrics`

```prometheus
# Counters
polyinsider_trades_received_total{market="all"}
polyinsider_trades_filtered_total{reason="low_value"}
polyinsider_signals_detected_total{type="FRESH_INSIDER"}
polyinsider_alerts_sent_total{destination="discord"}
polyinsider_rpc_calls_total{status="success"}
polyinsider_rpc_calls_total{status="error"}

# Gauges
polyinsider_websocket_connected{endpoint="polymarket"}
polyinsider_worker_pool_active
polyinsider_burst_tracker_addresses

# Histograms
polyinsider_rpc_latency_seconds
polyinsider_alert_latency_seconds  # Time from trade to Discord alert
polyinsider_db_write_latency_seconds
```

---

## 9. Error Handling & Resilience

### 9.1 WebSocket Reconnection

```go
const (
    InitialBackoff = 1 * time.Second
    MaxBackoff     = 60 * time.Second
    BackoffFactor  = 2.0
    JitterPercent  = 0.2
)
```

**Logic:**
1. On disconnect, wait `backoff` seconds
2. Attempt reconnect
3. If fail, `backoff = min(backoff * 2, MaxBackoff)` + jitter
4. If success, reset `backoff = InitialBackoff`

### 9.2 Heartbeat Monitor

If no WebSocket message received in 60 seconds:
1. Log warning
2. Send ping frame
3. If no pong in 10 seconds, force reconnect

### 9.3 Graceful Shutdown

On `SIGINT` / `SIGTERM`:
1. Stop accepting new trades
2. Drain worker pool (wait up to 10 seconds)
3. Flush pending DB writes
4. Flush pending alerts
5. Close DB connection
6. Exit

---

## 10. Directory Structure

```
polyinsider/
├── cmd/
│   └── engine/
│       └── main.go              # Entry point, wiring ✅
├── internal/
│   ├── config/
│   │   └── config.go            # Env loading, validation ✅
│   ├── ingest/
│   │   ├── websocket.go         # WS connection, reconnect logic ✅
│   │   ├── parser.go            # JSON deserialization ✅
│   │   └── markets.go           # Gamma API client for active markets ✅
│   ├── enricher/
│   │   ├── rpc.go               # Alchemy/RPC client (TODO)
│   │   └── cache.go             # Nonce cache (TODO)
│   ├── detector/
│   │   ├── signals.go           # Signal detection logic (TODO)
│   │   └── burst.go             # In-memory burst tracker (TODO)
│   ├── store/
│   │   ├── sqlite.go            # DB operations (TODO)
│   │   └── models.go            # Trade, Alert structs ✅
│   ├── alert/
│   │   ├── discord.go           # Webhook client (TODO)
│   │   ├── formatter.go         # Message formatting (TODO)
│   │   └── batcher.go           # Alert batching logic (TODO)
│   └── metrics/
│       └── prometheus.go        # Metrics registration (TODO)
├── data/                        # SQLite database directory (gitignored)
├── bin/                         # Compiled binaries (gitignored)
├── .env.example                 # Template for .env ✅
├── .env                         # Local config (gitignored)
├── .gitignore                   # ✅
├── go.mod                       # ✅
├── go.sum                       # ✅
├── Makefile                     # Build, run, test commands ✅
└── docs/
    ├── PROJ.md                  # Project spec (this file) ✅
    └── TECH.md                  # Technical data flow docs ✅
```

---

## 11. Known Constraints & Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| **Polygon RPC Rate Limits** | Missed nonce enrichment | Aggressive filtering (>$2k), fallback RPC, caching |
| **WebSocket Instability** | Missed trades | Heartbeat monitor, exponential backoff reconnect ✅ |
| **Nonce ≠ True Freshness** | False positives | Nonce only reflects Polygon mainnet. A "fresh" wallet could be a whale's 10th chain. *Accept as limitation in Phase 1.* |
| **USDC Decimal Precision** | Incorrect value calculation | Verify if API `size` is raw (6 decimals) or already scaled. **Must test.** |
| **State Loss on Restart** | Missed Panic Burst signals | Acceptable for Phase 1. Document restart procedure. |
| **Discord Rate Limits** | Delayed alerts | Batching (30s window), respect 429 responses |
| **Market Channel Limitations** | No maker/taker addresses | Market channel only provides orderbook data. Consider on-chain monitoring or REST API polling for full trade details. ⚠️ *Discovered Jan 2026* |
| **Token ID Requirement** | Must specify subscriptions | Empty `assets_ids` array doesn't subscribe to all markets. Must fetch active markets first. ✅ *Solved via Gamma API* |

---

## 12. Development Milestones

### Milestone 1: Skeleton & Config ✅ (Complete)
- [x] Initialize Go module
- [x] Set up directory structure
- [x] Implement config loading from `.env`
- [x] Basic logging setup (slog with structured output)

### Milestone 2: WebSocket Ingestion ✅ (Complete)
- [x] Connect to Polymarket WebSocket (`/ws/market` endpoint)
- [x] Fetch active markets from Gamma API
- [x] Subscribe to market token IDs
- [x] Parse `book` and `price_change` events
- [x] Implement reconnection with exponential backoff
- [x] Heartbeat monitoring

### Milestone 3: Signal Detection
- [ ] Implement value filter (>$2k)
- [ ] Implement Whale detection (>$50k)
- [ ] Implement Burst tracker (in-memory)
- [ ] Unit tests for detection logic

### Milestone 4: RPC Enrichment
- [ ] Alchemy client with `eth_getTransactionCount`
- [ ] Nonce caching (5 min TTL)
- [ ] Fallback to public RPC
- [ ] Fresh Insider detection

### Milestone 5: Persistence
- [ ] SQLite schema creation
- [ ] Batch writer implementation
- [ ] Trade insertion
- [ ] Alert logging

### Milestone 6: Alerting
- [ ] Discord webhook client
- [ ] Rich embed formatting
- [ ] Alert batching (30s window)
- [ ] Cooldown tracking

### Milestone 7: Observability
- [ ] Prometheus metrics endpoint
- [ ] Key counters and histograms
- [ ] Structured logging polish

### Milestone 8: Hardening
- [ ] Graceful shutdown (partial - signal handling done)
- [ ] Error recovery testing
- [ ] End-to-end test with live data

---

## 13. Future Considerations (Out of Scope)

These items are intentionally deferred but the architecture should not preclude them:

1. **Redis Pub/Sub Bridge** — For Phase 2 Python sidecar
2. **Multiple Discord Channels** — Route by signal severity
3. **Web Dashboard** — Real-time trade visualization
4. **Telegram Bot** — Alternative notification channel
5. **Backfill Mode** — Replay historical trades for testing signals
6. **Multi-chain Support** — Ethereum mainnet nonce check for extra freshness signal

---

## Appendix A: Discord Embed Format

```json
{
  "embeds": [{
    "title": "🔴 Fresh Insider Detected",
    "color": 15158332,
    "fields": [
      {"name": "Wallet", "value": "`0x1234...abcd`", "inline": true},
      {"name": "Nonce", "value": "2", "inline": true},
      {"name": "Value", "value": "$5,420.00", "inline": true},
      {"name": "Market", "value": "Will X happen by Y?", "inline": false},
      {"name": "Side", "value": "BUY YES @ 0.65", "inline": true}
    ],
    "timestamp": "2025-01-04T14:32:01.000Z",
    "footer": {"text": "Polyinsider v1.0"}
  }]
}
```

---

## Appendix B: Sample `.env.example`

```bash
# Polymarket WebSocket
POLYMARKET_WS_URL=wss://ws-subscriptions-clob.polymarket.com/ws/

# Alchemy RPC (get key at https://alchemy.com)
ALCHEMY_API_KEY=
ALCHEMY_URL=https://polygon-mainnet.g.alchemy.com/v2/
FALLBACK_RPC_URL=https://polygon-rpc.com

# Detection Thresholds
MIN_VALUE_USD=2000
WHALE_VALUE_USD=50000
FRESH_WALLET_NONCE=5
BURST_COUNT=3
BURST_WINDOW_SECONDS=60

# Discord Alerts
DISCORD_WEBHOOK_URL=
ALERT_BATCH_SECONDS=30
ALERT_COOLDOWN_MINUTES=60

# Database
DB_PATH=./data/trades.db

# Performance
WORKER_COUNT=5

# Metrics
PROMETHEUS_PORT=9090

# Logging
LOG_LEVEL=INFO
```
