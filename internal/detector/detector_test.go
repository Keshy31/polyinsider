package detector

import (
	"testing"
	"time"

	"github.com/polyinsider/engine/internal/config"
	"github.com/polyinsider/engine/internal/store"
)

func TestDetector(t *testing.T) {
	cfg := &config.Config{
		MinValueUSD:      2000,
		WhaleValueUSD:    50000,
		FreshWalletNonce: 5,
		BurstCount:       3,
		BurstWindow:      60 * time.Second,
	}

	d := NewDetector(cfg)

	// Test Case 1: Whale
	whaleTrade := store.Trade{
		ID:           "whale-1",
		ValueUSD:     55000,
		MakerAddress: "0xWhale",
	}
	signals := d.Detect(whaleTrade, -1)
	if len(signals) != 1 || signals[0].SignalType != store.SignalWhale {
		t.Errorf("Expected 1 Whale signal, got %v", signals)
	}

	// Test Case 2: Fresh Insider
	freshTrade := store.Trade{
		ID:           "fresh-1",
		ValueUSD:     5000, // > 2000
		MakerAddress: "0xFresh",
	}
	signals = d.Detect(freshTrade, 2) // Nonce 2 < 5
	if len(signals) != 1 || signals[0].SignalType != store.SignalFreshInsider {
		t.Errorf("Expected 1 Fresh Insider signal, got %v", signals)
	}
	if signals[0].Nonce != 2 {
		t.Errorf("Expected Nonce 2, got %d", signals[0].Nonce)
	}

	// Test Case 3: Fresh Insider but too small
	smallFreshTrade := store.Trade{
		ID:           "small-fresh",
		ValueUSD:     1000, // < 2000
		MakerAddress: "0xSmall",
	}
	signals = d.Detect(smallFreshTrade, 2)
	if len(signals) != 0 {
		t.Errorf("Expected 0 signals for small trade, got %v", signals)
	}

	// Test Case 4: Burst
	burstAddr := "0xBurst"
	burstTrade := store.Trade{
		ID:           "burst-trade",
		ValueUSD:     100,
		MakerAddress: burstAddr,
	}

	// First trade
	signals = d.Detect(burstTrade, -1)
	if len(signals) != 0 {
		t.Errorf("Expected 0 signals on first burst trade")
	}

	// Second trade
	signals = d.Detect(burstTrade, -1)
	if len(signals) != 0 {
		t.Errorf("Expected 0 signals on second burst trade")
	}

	// Third trade (should trigger)
	signals = d.Detect(burstTrade, -1)
	if len(signals) != 1 || signals[0].SignalType != store.SignalPanicBurst {
		t.Errorf("Expected Panic Burst signal on third trade, got %v", signals)
	}
}

func TestShouldEnrich(t *testing.T) {
	cfg := &config.Config{
		MinValueUSD: 2000,
	}
	d := NewDetector(cfg)

	// Should enrich high value with address
	trade := store.Trade{
		ValueUSD:     2500,
		MakerAddress: "0x123",
	}
	if !d.ShouldEnrich(trade) {
		t.Error("Expected ShouldEnrich to return true for high value trade with address")
	}

	// Should NOT enrich low value
	lowVal := store.Trade{
		ValueUSD:     1000,
		MakerAddress: "0x123",
	}
	if d.ShouldEnrich(lowVal) {
		t.Error("Expected ShouldEnrich to return false for low value trade")
	}

	// Should NOT enrich if no address (even if high value)
	noAddr := store.Trade{
		ValueUSD:     5000,
		MakerAddress: "",
	}
	if d.ShouldEnrich(noAddr) {
		t.Error("Expected ShouldEnrich to return false for trade with no address")
	}
}

