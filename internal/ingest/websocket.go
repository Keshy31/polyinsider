package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/polyinsider/engine/internal/store"
)

// Reconnection constants per spec Section 9.1
const (
	InitialBackoff = 1 * time.Second
	MaxBackoff     = 60 * time.Second
	BackoffFactor  = 2.0
	JitterPercent  = 0.2

	// Heartbeat constants per spec Section 9.2
	HeartbeatTimeout = 60 * time.Second
	PongTimeout      = 10 * time.Second

	// Write timeout
	WriteTimeout = 10 * time.Second
)

// Listener manages WebSocket connection to Polymarket.
type Listener struct {
	url        string
	tradeChan  chan<- store.Trade
	conn       *websocket.Conn
	connMu     sync.Mutex
	backoff    time.Duration
	lastMsg    time.Time
	lastMsgMu  sync.RWMutex
	stopChan   chan struct{}
	wg         sync.WaitGroup
	assetIDs   []string
	assetIDsMu sync.RWMutex
}

// NewListener creates a new WebSocket listener.
func NewListener(url string, tradeChan chan<- store.Trade) *Listener {
	return &Listener{
		url:       url,
		tradeChan: tradeChan,
		backoff:   InitialBackoff,
		stopChan:  make(chan struct{}),
		assetIDs:  []string{},
	}
}

// SetAssetIDs sets the asset IDs to subscribe to.
func (l *Listener) SetAssetIDs(ids []string) {
	l.assetIDsMu.Lock()
	defer l.assetIDsMu.Unlock()
	l.assetIDs = ids
}

// Start begins the WebSocket listener with automatic reconnection.
func (l *Listener) Start(ctx context.Context) {
	l.wg.Add(1)
	go l.runLoop(ctx)

	l.wg.Add(1)
	go l.heartbeatMonitor(ctx)
}

// Stop gracefully shuts down the listener.
func (l *Listener) Stop() {
	close(l.stopChan)
	l.closeConnection()
	l.wg.Wait()
}

// runLoop handles connection, reading, and reconnection.
func (l *Listener) runLoop(ctx context.Context) {
	defer l.wg.Done()

	for {
		select {
		case <-ctx.Done():
			slog.Info("ws_loop_stopping", "reason", "context cancelled")
			return
		case <-l.stopChan:
			slog.Info("ws_loop_stopping", "reason", "stop signal")
			return
		default:
		}

		// Attempt connection
		if err := l.connect(ctx); err != nil {
			slog.Error("ws_connect_failed", "error", err, "backoff", l.backoff)
			l.waitBackoff(ctx)
			continue
		}

		// Read messages until error
		if err := l.readLoop(ctx); err != nil {
			slog.Warn("ws_read_error", "error", err)
		}

		l.closeConnection()

		// Check if we should stop
		select {
		case <-ctx.Done():
			return
		case <-l.stopChan:
			return
		default:
			l.waitBackoff(ctx)
		}
	}
}

// connect establishes WebSocket connection and subscribes to trades.
func (l *Listener) connect(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	headers := http.Header{}
	headers.Set("Origin", "https://polymarket.com")

	// Ensure URL has /market path for the market channel
	url := l.url
	if !strings.HasSuffix(url, "/market") && !strings.HasSuffix(url, "/user") {
		url = strings.TrimSuffix(url, "/") + "/market"
	}

	conn, resp, err := dialer.DialContext(ctx, url, headers)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("dial failed with status %d: %w", resp.StatusCode, err)
		}
		return fmt.Errorf("dial failed: %w", err)
	}

	l.connMu.Lock()
	l.conn = conn
	l.connMu.Unlock()

	// Reset backoff on successful connection
	l.backoff = InitialBackoff

	slog.Info("ws_connected", "endpoint", url)

	// Subscribe to market channel
	// Note: Empty assets_ids may subscribe to all, or we may need to fetch market IDs
	if err := l.subscribe(); err != nil {
		return fmt.Errorf("subscribe failed: %w", err)
	}

	l.updateLastMsg()
	return nil
}

// subscribe sends subscription message for the market channel.
func (l *Listener) subscribe() error {
	l.assetIDsMu.RLock()
	assetIDs := l.assetIDs
	l.assetIDsMu.RUnlock()

	// Subscribe to the market channel
	// Format based on polymarket-websocket-client:
	// {"type": "market", "assets_ids": [...]}
	msg := map[string]interface{}{
		"type":       "market",
		"assets_ids": assetIDs,
	}

	l.connMu.Lock()
	defer l.connMu.Unlock()

	if l.conn == nil {
		return fmt.Errorf("connection is nil")
	}

	l.conn.SetWriteDeadline(time.Now().Add(WriteTimeout))
	if err := l.conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("failed to send subscribe message: %w", err)
	}

	slog.Info("ws_subscribed", "channel", "market", "asset_count", len(assetIDs))
	return nil
}

// readLoop reads messages from the WebSocket.
func (l *Listener) readLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-l.stopChan:
			return nil
		default:
		}

		l.connMu.Lock()
		conn := l.conn
		l.connMu.Unlock()

		if conn == nil {
			return fmt.Errorf("connection is nil")
		}

		// Set read deadline
		conn.SetReadDeadline(time.Now().Add(HeartbeatTimeout + PongTimeout))

		_, message, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read error: %w", err)
		}

		l.updateLastMsg()

		// Parse and dispatch trades
		l.handleMessage(message)
	}
}

// handleMessage parses a message and dispatches trades.
func (l *Listener) handleMessage(data []byte) {
	trades, msgType, err := ParseMessage(data)
	if err != nil {
		slog.Debug("ws_parse_error", "error", err, "raw", string(data))
		return
	}

	// Log non-trade messages at debug level
	if len(trades) == 0 {
		if msgType != "" {
			slog.Debug("ws_message", "type", msgType)
		}
		return
	}

	// Dispatch trades to channel
	for _, trade := range trades {
		select {
		case l.tradeChan <- trade:
			slog.Debug("trade_received",
				"market", truncate(trade.MarketID, 16),
				"maker", truncate(trade.MakerAddress, 10),
				"size", trade.Size,
				"price", trade.Price,
				"value_usd", trade.ValueUSD,
			)
		default:
			slog.Warn("trade_channel_full", "dropped_trade", trade.ID)
		}
	}
}

// heartbeatMonitor checks for connection health.
func (l *Listener) heartbeatMonitor(ctx context.Context) {
	defer l.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-l.stopChan:
			return
		case <-ticker.C:
			l.checkHeartbeat()
		}
	}
}

// checkHeartbeat verifies we've received messages recently.
func (l *Listener) checkHeartbeat() {
	l.lastMsgMu.RLock()
	lastMsg := l.lastMsg
	l.lastMsgMu.RUnlock()

	if lastMsg.IsZero() {
		return
	}

	elapsed := time.Since(lastMsg)
	if elapsed > HeartbeatTimeout {
		slog.Warn("ws_heartbeat_timeout", "elapsed", elapsed)

		// Send ping
		l.connMu.Lock()
		conn := l.conn
		l.connMu.Unlock()

		if conn != nil {
			conn.SetWriteDeadline(time.Now().Add(WriteTimeout))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				slog.Warn("ws_ping_failed", "error", err)
				l.closeConnection()
			}
		}
	}
}

// updateLastMsg updates the last message timestamp.
func (l *Listener) updateLastMsg() {
	l.lastMsgMu.Lock()
	l.lastMsg = time.Now()
	l.lastMsgMu.Unlock()
}

// closeConnection safely closes the WebSocket connection.
func (l *Listener) closeConnection() {
	l.connMu.Lock()
	defer l.connMu.Unlock()

	if l.conn != nil {
		l.conn.Close()
		l.conn = nil
		slog.Info("ws_disconnected")
	}
}

// waitBackoff waits for the backoff duration with jitter.
func (l *Listener) waitBackoff(ctx context.Context) {
	// Add jitter
	jitter := time.Duration(float64(l.backoff) * JitterPercent * (rand.Float64()*2 - 1))
	wait := l.backoff + jitter

	slog.Debug("ws_waiting_backoff", "duration", wait)

	select {
	case <-ctx.Done():
	case <-l.stopChan:
	case <-time.After(wait):
	}

	// Increase backoff for next attempt
	l.backoff = time.Duration(float64(l.backoff) * BackoffFactor)
	if l.backoff > MaxBackoff {
		l.backoff = MaxBackoff
	}
}

// truncate shortens a string for logging.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// SubscriptionMessage represents a subscription request.
type SubscriptionMessage struct {
	Type      string   `json:"type"`
	Channel   string   `json:"channel"`
	AssetsIDs []string `json:"assets_ids,omitempty"`
}

// NewSubscriptionMessage creates a new subscription message.
func NewSubscriptionMessage(channel string, assetIDs []string) *SubscriptionMessage {
	return &SubscriptionMessage{
		Type:      "subscribe",
		Channel:   channel,
		AssetsIDs: assetIDs,
	}
}

// ToJSON serializes the subscription message.
func (s *SubscriptionMessage) ToJSON() ([]byte, error) {
	return json.Marshal(s)
}

