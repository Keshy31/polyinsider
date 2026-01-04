# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Polyinsider is a low-latency surveillance system for detecting insider trading and anomalous whale activity on Polymarket (Polygon Network). The system is built in Go with a philosophy of **speed first, intelligence second**, targeting <500ms latency from trade detection to alert.

## Build & Run Commands

```bash
# Build the application
make build

# Run (builds first if needed)
make run

# Run in development mode (no binary build)
make dev

# Run tests
make test

# Download and tidy dependencies
make deps

# Clean build artifacts
make clean

# Initialize data directory
make init
```

**Binary location**: `bin/engine`

## Architecture Overview

### Core Pipeline

The system follows a concurrent pipeline architecture:

```
WebSocket → Parser → Trade Channel → Worker Pool → Signal Detector → Suspect Channel → TUI/Alerts
                                         ↓
                                    Metrics Tracker
```

### Key Components

1. **Ingestion Layer** (`internal/ingest/`)
   - `websocket.go`: WebSocket connection with exponential backoff reconnection (1s→60s)
   - `markets.go`: Gamma API client to fetch active markets and extract token IDs
   - `parser.go`: JSON message parser supporting multiple event formats (book events, price changes)
   - `trades_api.go`: Optional REST API poller (requires authentication, graceful failure)

2. **Detection Layer** (`internal/detector/`)
   - `signals.go`: Signal detection logic (Fresh Insider, Whale, Panic Burst, Price Shock)
   - `burst.go`: In-memory burst tracker with TTL map for detecting rapid trading patterns

3. **Metrics Layer** (`internal/metrics/`)
   - `tracker.go`: Thread-safe metrics aggregation for TUI and future Prometheus export

4. **UI Layer** (`internal/ui/`)
   - `app.go`: TUI application coordinator
   - Five view components: Market Overview, Signal Alerter, Live Trades, Stats Dashboard, Top Movers

5. **Storage Layer** (`internal/store/`)
   - `models.go`: Core data structures (Trade, Suspect, Alert)

### Concurrency Model

- **Main goroutine**: Coordinates startup and waits for shutdown signal
- **Listener goroutine**: Reads from WebSocket, parses messages, sends to `tradeChan`
- **Heartbeat goroutine**: Monitors WebSocket connection health (60s timeout)
- **Worker pool** (5-10 goroutines): Processes trades, detects signals, updates metrics
- **TUI goroutine**: Renders interface (if enabled)
- **Cleanup goroutine**: Periodic metrics cleanup every 5 minutes

**Channels:**
- `tradeChan`: Buffered channel (cap=1000) for trades from WebSocket to workers
- `suspectChan`: Buffered channel (cap=100) for detected signals to TUI/alerts

## Configuration

Configuration is loaded from environment variables with fallback to `.env` file. See `.env.example` for template.

**Priority**: Environment variables > `.env` file > hardcoded defaults

### Critical Configuration Values

- `POLYMARKET_WS_URL`: WebSocket endpoint (default: `wss://ws-subscriptions-clob.polymarket.com/ws/`)
- `ENABLE_TUI`: Enable terminal UI (default: `true`)
- `MIN_VALUE_USD`: Minimum trade value to process (default: `2000`)
- `WHALE_VALUE_USD`: Whale detection threshold (default: `50000`)
- `WORKER_COUNT`: Number of concurrent workers (default: `5`)
- `LOG_LEVEL`: Logging level (DEBUG/INFO/WARN/ERROR, default: `INFO`)

## Signal Detection Logic

### 1. Fresh Insider 🔴
```
IF value_usd > $2,000 AND wallet_nonce < 5 THEN ALERT
```
**Status**: Partially implemented (nonce enrichment pending RPC integration)

### 2. Whale 🐋
```
IF value_usd > $50,000 THEN ALERT
```
**Status**: Fully functional

### 3. Panic Burst ⚡
```
IF trades_from_address_in_last_60s >= 3 THEN ALERT
```
**Status**: Fully functional (in-memory tracking)

### 4. Price Shock 📈
```
IF price_change_pct >= 5% THEN ALERT
```
**Status**: Fully functional

## Data Flow Details

### WebSocket Message Processing

The system subscribes to specific Polymarket token IDs (not all markets):
1. Fetch active markets from `https://gamma-api.polymarket.com/markets`
2. Extract `clobTokenIds` from each market (typically 2 per market: YES/NO)
3. Subscribe to all token IDs via WebSocket
4. Parse incoming `book` events (orderbook snapshots with `last_trade_price`)

**Important**: The market channel provides orderbook data, NOT individual trade events with maker/taker addresses. Trade reconstruction uses `last_trade_price` field.

### Message Parsing Strategy

Parser attempts multiple formats in order:
1. Array of BookEvent objects
2. Single BookEvent
3. WSMessage wrapper
4. last_trade_price event
5. trade event

This flexibility handles API schema variations.

## Logging Conventions

Uses structured logging (slog) with custom time format:

```
time="2026-01-04 14:32:01" level=INFO msg=event_name key1=value1 key2=value2
```

**Key events to look for:**
- `polyinsider_starting`: Startup with version
- `config_loaded`: Configuration dump (secrets masked)
- `fetched_active_markets`: Market discovery results
- `ws_connected`: WebSocket connection established
- `trade_received` (DEBUG): Individual trade events
- `signal_detected` (DEBUG): Detection triggers
- `shutdown_signal_received`: Graceful shutdown initiated

## Important Implementation Notes

### Wallet Address Limitation
The WebSocket market channel does NOT provide wallet addresses (maker/taker). This impacts:
- Fresh Insider detection requires on-chain RPC calls (not yet implemented)
- Whale detection works but cannot attribute to specific wallets
- Burst detection limited to aggregated market activity

**Future solution**: Implement on-chain event monitoring or authenticated user channel.

### Graceful Shutdown
Signal handling for SIGINT/SIGTERM:
1. Cancel context to stop all goroutines
2. Stop WebSocket listener
3. Drain remaining trades from channel (5s timeout)
4. Exit cleanly

### Error Handling Patterns
- WebSocket disconnections trigger exponential backoff reconnection
- RPC errors logged but don't crash the system
- Channel overflow warnings logged with dropped trade IDs
- JSON parse errors logged at DEBUG level (non-critical)

## Code Organization Principles

1. **Package structure follows layers**: `ingest`, `detector`, `store`, `ui`, `metrics`, `config`
2. **Channels for concurrency**: Prefer channels over mutexes for goroutine communication
3. **Context for cancellation**: All long-running goroutines check `ctx.Done()`
4. **Structured logging**: Always use key-value pairs, never string interpolation
5. **Configuration validation**: Config struct has `Validate()` method called at startup

## Testing

Run tests with:
```bash
make test
```

Current test coverage:
- Detector logic tests in `internal/detector/detector_test.go`
- (Additional tests needed for websocket parsing, metrics, and UI components)

## Development Workflow

1. Modify code in `internal/` or `cmd/engine/`
2. Run `make dev` for quick iteration (no binary build)
3. Check logs with `LOG_LEVEL=DEBUG` for detailed tracing
4. Use `make build` before committing to verify clean compilation
5. Test TUI with `ENABLE_TUI=true`, background mode with `ENABLE_TUI=false`

## Known Constraints

- **State is ephemeral**: Burst tracker and metrics reset on restart (acceptable for Phase 1)
- **No persistence layer**: SQLite implementation pending (schema documented in docs/PROJ.md)
- **No alerting**: Discord webhook client not yet implemented
- **REST API authentication**: Trade polling will fail with 401 (graceful, falls back to WebSocket)
- **Nonce enrichment pending**: Alchemy RPC integration incomplete

## Dependencies

Key external packages:
- `github.com/joho/godotenv`: Environment variable loading
- `github.com/gorilla/websocket`: WebSocket client
- `github.com/rivo/tview`: Terminal UI framework

Install with `make deps` or `go mod download`.

## File Locations

- **Configuration**: `.env` (gitignored), `.env.example` (template)
- **Data directory**: `./data/` (gitignored, create with `make init`)
- **Binary output**: `./bin/engine` (gitignored)
- **Documentation**: `docs/PROJ.md` (full spec), `docs/TECH.md` (technical details), `docs/TUI_USAGE.md` (UI guide)
