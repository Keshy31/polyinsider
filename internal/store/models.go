// Package store provides data models and database operations.
package store

import "time"

// Trade represents a single trade event from Polymarket.
type Trade struct {
	// ID is a unique identifier for this trade record
	ID string

	// MarketID is the market/condition token ID
	MarketID string

	// AssetID is the specific outcome token ID
	AssetID string

	// MakerAddress is the wallet that created the order
	MakerAddress string

	// TakerAddress is the wallet that filled the order (may be empty)
	TakerAddress string

	// Side is BUY or SELL
	Side string

	// Outcome is YES or NO (derived from asset)
	Outcome string

	// Size is the raw trade size (string to preserve precision)
	Size string

	// Price is the execution price (0-1 range for prediction markets)
	Price float64

	// ValueUSD is the calculated USD value of the trade
	ValueUSD float64

	// Timestamp is when the trade occurred
	Timestamp time.Time

	// TradeID is the original trade ID from Polymarket
	TradeID string

	// TransactionHash is the on-chain transaction hash (if available)
	TransactionHash string
}

// Signal types for detection
const (
	SignalFreshInsider = "FRESH_INSIDER"
	SignalWhale        = "WHALE"
	SignalPanicBurst   = "PANIC_BURST"
	SignalPriceShock   = "PRICE_SHOCK" // New signal for rapid price moves > 5%
)

// Suspect represents a trade that triggered a detection signal.
type Suspect struct {
	Trade      Trade
	SignalType string
	Nonce      int // Wallet transaction count (for FRESH_INSIDER)
	Meta       map[string]interface{} // Extra context (e.g., price delta)
}

// Alert represents a notification to be sent.
type Alert struct {
	ID            string
	TradeIDs      []string
	WalletAddress string
	SignalType    string
	Summary       string
	SentAt        time.Time
	Success       bool
}
