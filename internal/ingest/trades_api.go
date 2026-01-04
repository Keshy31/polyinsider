// Package ingest provides trade data polling functionality.
package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/polyinsider/engine/internal/store"
)

const (
	// CLOBAPIBaseURL is the Polymarket CLOB API endpoint
	CLOBAPIBaseURL = "https://clob.polymarket.com"
	// DefaultPollInterval is the default polling interval
	DefaultPollInterval = 3 * time.Second
)

// TradeAPIResponse represents the response from the CLOB trades endpoint.
type TradeAPIResponse struct {
	ID              string `json:"id"`
	Market          string `json:"market"`
	AssetID         string `json:"asset_id"`
	MakerAddress    string `json:"maker_address"`
	TakerAddress    string `json:"taker_address"`
	Side            string `json:"side"`
	Size            string `json:"size"`
	Price           string `json:"price"`
	Outcome         string `json:"outcome"`
	Timestamp       int64  `json:"timestamp"` // Unix timestamp in milliseconds
	TransactionHash string `json:"transaction_hash"`
	TradeID         string `json:"trade_id"`
}

// TradesPoller polls the Polymarket CLOB API for recent trades.
type TradesPoller struct {
	baseURL   string
	client    *http.Client
	interval  time.Duration
	tradeChan chan<- store.Trade
	lastPoll  time.Time
}

// NewTradesPoller creates a new TradesPoller.
func NewTradesPoller(baseURL string, interval time.Duration, tradeChan chan<- store.Trade) *TradesPoller {
	if baseURL == "" {
		baseURL = CLOBAPIBaseURL
	}
	if interval == 0 {
		interval = DefaultPollInterval
	}

	return &TradesPoller{
		baseURL:   baseURL,
		client:    &http.Client{Timeout: 10 * time.Second},
		interval:  interval,
		tradeChan: tradeChan,
		lastPoll:  time.Now().Add(-5 * time.Minute), // Start with 5 min lookback
	}
}

// Start begins polling for trades.
func (p *TradesPoller) Start(ctx context.Context) {
	slog.Info("starting_trades_poller", "base_url", p.baseURL, "interval", p.interval)
	
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// Initial fetch
	if err := p.poll(ctx); err != nil {
		slog.Warn("initial_poll_failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("trades_poller_stopped")
			return
		case <-ticker.C:
			if err := p.poll(ctx); err != nil {
				slog.Debug("poll_failed", "error", err)
			}
		}
	}
}

// poll fetches recent trades and sends them to the trade channel.
func (p *TradesPoller) poll(ctx context.Context) error {
	trades, err := p.fetchRecentTrades(ctx, p.lastPoll)
	if err != nil {
		return fmt.Errorf("fetch failed: %w", err)
	}

	if len(trades) > 0 {
		slog.Debug("trades_fetched", "count", len(trades))
		p.lastPoll = time.Now()

		for _, trade := range trades {
			select {
			case p.tradeChan <- trade:
				// Successfully sent
			default:
				slog.Warn("trade_channel_full_api", "dropped_trade", trade.ID)
			}
		}
	}

	return nil
}

// fetchRecentTrades fetches trades after the given timestamp.
func (p *TradesPoller) fetchRecentTrades(ctx context.Context, after time.Time) ([]store.Trade, error) {
	// Note: The exact endpoint structure needs to be verified with Polymarket's API
	// This is a best-effort implementation based on common REST API patterns
	// URL might be: /trades?after=TIMESTAMP&limit=100
	
	afterMs := after.UnixMilli()
	url := fmt.Sprintf("%s/trades?after=%d&limit=100", p.baseURL, afterMs)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// If endpoint doesn't exist or returns 404, log once and continue
		// This allows the system to work with just WebSocket data
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("endpoint not found (this is expected if using WebSocket only)")
		}
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var apiTrades []TradeAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiTrades); err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}

	// Convert API trades to store.Trade
	trades := make([]store.Trade, 0, len(apiTrades))
	for _, apiTrade := range apiTrades {
		trade := p.convertTrade(apiTrade)
		trades = append(trades, trade)
	}

	return trades, nil
}

// convertTrade converts a TradeAPIResponse to store.Trade.
func (p *TradesPoller) convertTrade(apiTrade TradeAPIResponse) store.Trade {
	price := parseFloatSafe(apiTrade.Price)
	size := parseFloatSafe(apiTrade.Size)
	valueUSD := price * size // Simplified calculation

	return store.Trade{
		ID:              fmt.Sprintf("api-%s", apiTrade.ID),
		MarketID:        apiTrade.Market,
		AssetID:         apiTrade.AssetID,
		MakerAddress:    apiTrade.MakerAddress,
		TakerAddress:    apiTrade.TakerAddress,
		Side:            apiTrade.Side,
		Outcome:         apiTrade.Outcome,
		Size:            apiTrade.Size,
		Price:           price,
		ValueUSD:        valueUSD,
		Timestamp:       time.UnixMilli(apiTrade.Timestamp),
		TradeID:         apiTrade.TradeID,
		TransactionHash: apiTrade.TransactionHash,
	}
}

// parseFloatSafe safely parses a string to float64, returning 0 on error.
func parseFloatSafe(s string) float64 {
	if s == "" {
		return 0
	}
	
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

