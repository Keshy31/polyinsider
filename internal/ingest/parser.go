// Package ingest handles WebSocket connection and message parsing from Polymarket.
package ingest

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/polyinsider/engine/internal/store"
)

// WSMessage represents the base structure of a WebSocket message.
type WSMessage struct {
	Type    string          `json:"type"`
	Channel string          `json:"channel,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// BookEvent represents an orderbook snapshot from the market channel.
// This is the actual format received from Polymarket WebSocket.
type BookEvent struct {
	Market         string `json:"market"`          // Condition ID
	AssetID        string `json:"asset_id"`        // Token ID
	Timestamp      string `json:"timestamp"`       // Unix timestamp in ms
	Hash           string `json:"hash"`            // Event hash
	EventType      string `json:"event_type"`      // "book", "price_change", etc.
	LastTradePrice string `json:"last_trade_price"` // Last executed trade price
	Bids           []struct {
		Price string `json:"price"`
		Size  string `json:"size"`
	} `json:"bids"`
	Asks []struct {
		Price string `json:"price"`
		Size  string `json:"size"`
	} `json:"asks"`
}

// TradeData represents trade data from the Polymarket WebSocket.
// The exact schema may vary, so we use flexible types.
type TradeData struct {
	ID              string `json:"id"`
	TradeID         string `json:"trade_id"`
	Market          string `json:"market"`
	AssetID         string `json:"asset_id"`
	Maker           string `json:"maker"`
	Taker           string `json:"taker"`
	MakerAddress    string `json:"maker_address"`
	TakerAddress    string `json:"taker_address"`
	Side            string `json:"side"`
	Size            string `json:"size"`
	Price           string `json:"price"`
	Outcome         string `json:"outcome"`
	Timestamp       string `json:"timestamp"`
	TransactionHash string `json:"transaction_hash"`
	MatchTime       string `json:"match_time"`
	Status          string `json:"status"`
}

// TradeEvent wraps trade data in the event structure.
type TradeEvent struct {
	Trades []TradeData `json:"trades"`
	// Single trade format
	TradeData
}

// LastTradePriceEvent represents the last_trade_price WebSocket event.
// This is the primary event for trade execution data from Polymarket.
type LastTradePriceEvent struct {
	Type    string `json:"type"`     // "last_trade_price"
	AssetID string `json:"asset_id"` // Token ID
	Price   string `json:"price"`    // Execution price
	Size    string `json:"size"`     // Trade size
	Side    string `json:"side"`     // BUY or SELL (if available)
	Maker   string `json:"maker"`    // Maker address (if available)
	Taker   string `json:"taker"`    // Taker address (if available)
}

// ParseMessage parses a raw WebSocket message and returns trades if present.
func ParseMessage(data []byte) ([]store.Trade, string, error) {
	// First, try to parse as an array of BookEvents (the actual format from Polymarket)
	var bookEvents []BookEvent
	if err := json.Unmarshal(data, &bookEvents); err == nil && len(bookEvents) > 0 {
		// Check if these are book events
		if bookEvents[0].EventType == "book" || bookEvents[0].EventType == "price_change" {
			trades := parseBookEvents(bookEvents)
			return trades, "book_array", nil
		}
	}

	// Try to parse as a single BookEvent
	var singleBook BookEvent
	if err := json.Unmarshal(data, &singleBook); err == nil && singleBook.EventType != "" {
		trades := parseBookEvents([]BookEvent{singleBook})
		return trades, singleBook.EventType, nil
	}

	// Try to parse as WSMessage wrapper
	var msg WSMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Handle last_trade_price events
	if msg.Type == "last_trade_price" {
		trades, err := parseLastTradePrice(data)
		if err != nil {
			return nil, msg.Type, err
		}
		return trades, msg.Type, nil
	}

	// Handle trade events
	if msg.Type == "trade" {
		trades, err := parseTrades(msg.Data)
		if err != nil {
			return nil, msg.Type, err
		}
		return trades, msg.Type, nil
	}

	// Return message type for other messages
	return nil, msg.Type, nil
}

// parseBookEvents extracts trade information from book events.
// The last_trade_price field in book events indicates recent trade activity.
func parseBookEvents(events []BookEvent) []store.Trade {
	var trades []store.Trade

	for _, event := range events {
		// Only create a "trade" record if there's a last_trade_price
		if event.LastTradePrice == "" || event.LastTradePrice == "0" {
			continue
		}

		price := parseFloat(event.LastTradePrice)
		if price == 0 {
			continue
		}

		// Create a trade record from the book event
		trade := store.Trade{
			ID:        fmt.Sprintf("book-%s-%s", event.AssetID[:min(8, len(event.AssetID))], event.Timestamp),
			MarketID:  event.Market,
			AssetID:   event.AssetID,
			Price:     price,
			Timestamp: parseTimestamp(event.Timestamp),
		}

		// Estimate value from orderbook depth (rough approximation)
		// In reality, we'd need actual trade size, but book events don't provide it
		// Mark as 0 so we know it's not a real trade value
		trade.ValueUSD = 0
		trade.Size = "book_update"

		trades = append(trades, trade)
	}

	return trades
}

// parseLastTradePrice parses a last_trade_price event.
func parseLastTradePrice(data []byte) ([]store.Trade, error) {
	var event LastTradePriceEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("failed to parse last_trade_price: %w", err)
	}

	if event.AssetID == "" {
		return nil, nil
	}

	trade := store.Trade{
		ID:           fmt.Sprintf("ltp-%s-%d", event.AssetID[:min(8, len(event.AssetID))], time.Now().UnixNano()),
		AssetID:      event.AssetID,
		MakerAddress: event.Maker,
		TakerAddress: event.Taker,
		Side:         event.Side,
		Size:         event.Size,
		Price:        parseFloat(event.Price),
		Timestamp:    time.Now(),
	}

	trade.ValueUSD = calculateValueUSD(trade.Size, trade.Price)

	return []store.Trade{trade}, nil
}

// parseTrades extracts trade data from the message payload.
func parseTrades(data json.RawMessage) ([]store.Trade, error) {
	if len(data) == 0 {
		return nil, nil
	}

	// Try parsing as array of trades first
	var tradesArray []TradeData
	if err := json.Unmarshal(data, &tradesArray); err == nil && len(tradesArray) > 0 {
		return convertTrades(tradesArray), nil
	}

	// Try parsing as trade event with trades array
	var event TradeEvent
	if err := json.Unmarshal(data, &event); err == nil {
		if len(event.Trades) > 0 {
			return convertTrades(event.Trades), nil
		}
		// Single trade in event
		if event.TradeData.ID != "" || event.TradeData.TradeID != "" {
			return convertTrades([]TradeData{event.TradeData}), nil
		}
	}

	// Try parsing as single trade
	var single TradeData
	if err := json.Unmarshal(data, &single); err == nil {
		if single.ID != "" || single.TradeID != "" || single.Market != "" {
			return convertTrades([]TradeData{single}), nil
		}
	}

	return nil, nil
}

// convertTrades converts TradeData to store.Trade.
func convertTrades(data []TradeData) []store.Trade {
	trades := make([]store.Trade, 0, len(data))

	for _, td := range data {
		trade := store.Trade{
			ID:              generateID(td),
			MarketID:        td.Market,
			AssetID:         td.AssetID,
			MakerAddress:    coalesce(td.MakerAddress, td.Maker),
			TakerAddress:    coalesce(td.TakerAddress, td.Taker),
			Side:            td.Side,
			Outcome:         td.Outcome,
			Size:            td.Size,
			Price:           parseFloat(td.Price),
			TradeID:         coalesce(td.TradeID, td.ID),
			TransactionHash: td.TransactionHash,
			Timestamp:       parseTimestamp(td.Timestamp, td.MatchTime),
		}

		// Calculate USD value (size * price for USDC markets)
		trade.ValueUSD = calculateValueUSD(trade.Size, trade.Price)

		trades = append(trades, trade)
	}

	return trades
}

// generateID creates a unique ID for the trade record.
func generateID(td TradeData) string {
	if td.ID != "" {
		return td.ID
	}
	if td.TradeID != "" {
		return td.TradeID
	}
	// Fallback: generate from available data
	return fmt.Sprintf("%s-%s-%s", td.Market, td.Maker, td.Timestamp)
}

// coalesce returns the first non-empty string.
func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// parseFloat safely parses a string to float64.
func parseFloat(s string) float64 {
	if s == "" {
		return 0
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// parseTimestamp tries multiple timestamp formats.
func parseTimestamp(values ...string) time.Time {
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02 15:04:05",
	}

	for _, v := range values {
		if v == "" {
			continue
		}

		// Try parsing as Unix timestamp (seconds or milliseconds)
		if ts, err := strconv.ParseInt(v, 10, 64); err == nil {
			if ts > 1e12 {
				// Milliseconds
				return time.UnixMilli(ts)
			}
			return time.Unix(ts, 0)
		}

		// Try standard formats
		for _, format := range formats {
			if t, err := time.Parse(format, v); err == nil {
				return t
			}
		}
	}

	return time.Now()
}

// calculateValueUSD computes the USD value of a trade.
// For Polymarket, size is typically in USDC (6 decimals).
func calculateValueUSD(sizeStr string, price float64) float64 {
	size := parseFloat(sizeStr)
	if size == 0 {
		return 0
	}

	// If size looks like raw USDC (large number), divide by 1e6
	if size > 1e6 {
		size = size / 1e6
	}

	// Value = size * price for buy, size * (1-price) for sell
	// Simplified: just use size as the USD value for now
	// The actual calculation depends on the trade type
	return size
}

