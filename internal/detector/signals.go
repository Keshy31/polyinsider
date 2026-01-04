package detector

import (
	"math"
	"sync"

	"github.com/polyinsider/engine/internal/config"
	"github.com/polyinsider/engine/internal/store"
)

// Detector applies rules to detect suspicious trading activity.
type Detector struct {
	cfg          *config.Config
	burstTracker *BurstTracker
	
	mu         sync.RWMutex
	lastPrices map[string]float64 // assetID -> last price
}

// NewDetector creates a new Detector.
func NewDetector(cfg *config.Config) *Detector {
	return &Detector{
		cfg:          cfg,
		burstTracker: NewBurstTracker(cfg.BurstWindow),
		lastPrices:   make(map[string]float64),
	}
}

// Detect analyzes a trade and returns any signals found.
// nonce should be -1 if not available/enriched yet.
func (d *Detector) Detect(trade store.Trade, nonce int) []store.Suspect {
	var suspects []store.Suspect

	// Check 1: Price Shock (Impact > 5%)
	// Must happen before we update lastPrices
	d.mu.Lock()
	lastPrice, exists := d.lastPrices[trade.AssetID]
	d.lastPrices[trade.AssetID] = trade.Price
	d.mu.Unlock()

	if exists && lastPrice > 0 {
		// Calculate percentage change: |new - old| / old
		delta := math.Abs(trade.Price - lastPrice)
		pctChange := delta / lastPrice

		// 5% threshold (0.05)
		if pctChange >= 0.05 {
			suspects = append(suspects, store.Suspect{
				Trade:      trade,
				SignalType: store.SignalPriceShock,
				Nonce:      nonce,
				Meta: map[string]interface{}{
					"prev_price": lastPrice,
					"new_price":  trade.Price,
					"pct_change": pctChange,
				},
			})
		}
	}

	// Check 2: Whale
	// IF value_usd > 50000 THEN ALERT
	if trade.ValueUSD >= d.cfg.WhaleValueUSD {
		suspects = append(suspects, store.Suspect{
			Trade:      trade,
			SignalType: store.SignalWhale,
			Nonce:      nonce,
		})
	}

	// Check 3: Fresh Insider
	// IF value_usd > 2000 AND wallet_nonce < 5 THEN ALERT
	// We only check this if nonce is provided (>= 0)
	if nonce >= 0 && trade.ValueUSD >= d.cfg.MinValueUSD {
		if nonce <= d.cfg.FreshWalletNonce {
			suspects = append(suspects, store.Suspect{
				Trade:      trade,
				SignalType: store.SignalFreshInsider,
				Nonce:      nonce,
			})
		}
	}

	// Check 4: Panic Burst
	// IF trades_from_address_in_last_60s >= 3 THEN ALERT
	if trade.MakerAddress != "" {
		count := d.burstTracker.Record(trade.MakerAddress)
		if count >= d.cfg.BurstCount {
			suspects = append(suspects, store.Suspect{
				Trade:      trade,
				SignalType: store.SignalPanicBurst,
				Nonce:      nonce,
			})
		}
	}

	return suspects
}

// ShouldEnrich checks if a trade qualifies for expensive RPC enrichment (nonce check).
func (d *Detector) ShouldEnrich(trade store.Trade) bool {
	// Only enrich if value is high enough to be a potential Fresh Insider
	// and we have a Maker Address
	return trade.ValueUSD >= d.cfg.MinValueUSD && trade.MakerAddress != ""
}
