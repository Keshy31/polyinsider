package detector

import (
	"sync"
	"time"
)

// BurstTracker tracks trade frequency per address to detect panic bursts.
type BurstTracker struct {
	mu     sync.RWMutex
	trades map[string][]time.Time
	window time.Duration
}

// NewBurstTracker creates a new BurstTracker with the specified window.
func NewBurstTracker(window time.Duration) *BurstTracker {
	return &BurstTracker{
		trades: make(map[string][]time.Time),
		window: window,
	}
}

// Record adds a trade for the given address and returns the number of trades
// within the window (including the new one).
func (b *BurstTracker) Record(address string) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-b.window)

	// Get existing timestamps
	timestamps := b.trades[address]
	
	// Filter out old timestamps
	validIdx := 0
	for i, t := range timestamps {
		if t.After(cutoff) {
			validIdx = i
			break
		}
		// If we reach the end and none are valid
		if i == len(timestamps)-1 && !t.After(cutoff) {
			validIdx = len(timestamps)
		}
	}

	// Slice off old ones
	if validIdx > 0 {
		timestamps = timestamps[validIdx:]
	}

	// Add new timestamp
	timestamps = append(timestamps, now)
	b.trades[address] = timestamps

	return len(timestamps)
}

// Cleanup removes entries for addresses with no recent trades.
// Should be called periodically to prevent memory leaks.
func (b *BurstTracker) Cleanup() {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-b.window)

	for addr, timestamps := range b.trades {
		// Check last timestamp (most recent)
		if len(timestamps) == 0 || !timestamps[len(timestamps)-1].After(cutoff) {
			delete(b.trades, addr)
		}
	}
}

