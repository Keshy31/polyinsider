// Package metrics provides real-time metrics tracking for the system.
package metrics

import (
	"sync"
	"time"
)

// PricePoint represents a price at a specific time.
type PricePoint struct {
	Price     float64
	Timestamp time.Time
}

// MarketActivity tracks activity for a single market.
type MarketActivity struct {
	MarketID    string
	Question    string
	TradeCount  int
	Volume      float64
	LastPrice   float64
	PricePoints []PricePoint
	LastUpdate  time.Time
}

// MetricsSnapshot is a point-in-time view of metrics.
type MetricsSnapshot struct {
	TradesTotal       int64
	HighValueTrades   int64
	SignalsByType     map[string]int64
	TradeRate         float64 // trades per second
	MarketActivities  map[string]*MarketActivity
	TopMovers         []MoverStats
	Uptime            time.Duration
	WebSocketStatus   string
	RESTAPILastPoll   time.Time
	ChannelBufferUsed int
	ChannelBufferCap  int
}

// MoverStats represents a market with significant activity.
type MoverStats struct {
	MarketID     string
	Question     string
	PriceChange  float64 // percentage
	Volume       float64
	TradeCount   int
	CurrentPrice float64
}

// MetricsTracker provides thread-safe metrics tracking.
type MetricsTracker struct {
	mu                sync.RWMutex
	tradesTotal       int64
	highValueTrades   int64
	signalsByType     map[string]int64
	priceHistory      map[string][]PricePoint // marketID -> price history
	marketActivity    map[string]*MarketActivity
	startTime         time.Time
	lastTradeTime     time.Time
	tradeTimestamps   []time.Time // for rate calculation
	wsStatus          string
	restLastPoll      time.Time
	channelBufferUsed int
	channelBufferCap  int
}

// NewMetricsTracker creates a new MetricsTracker.
func NewMetricsTracker() *MetricsTracker {
	return &MetricsTracker{
		signalsByType:   make(map[string]int64),
		priceHistory:    make(map[string][]PricePoint),
		marketActivity:  make(map[string]*MarketActivity),
		startTime:       time.Now(),
		tradeTimestamps: make([]time.Time, 0, 1000),
		wsStatus:        "disconnected",
	}
}

// IncrementTrades increments the total trade counter.
func (m *MetricsTracker) IncrementTrades() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.tradesTotal++
	m.lastTradeTime = time.Now()
	
	// Add to timestamps for rate calculation
	m.tradeTimestamps = append(m.tradeTimestamps, m.lastTradeTime)
	
	// Keep only last 60 seconds of timestamps
	cutoff := m.lastTradeTime.Add(-60 * time.Second)
	validIdx := 0
	for i, ts := range m.tradeTimestamps {
		if ts.After(cutoff) {
			validIdx = i
			break
		}
	}
	if validIdx > 0 {
		m.tradeTimestamps = m.tradeTimestamps[validIdx:]
	}
}

// IncrementHighValue increments the high-value trade counter.
func (m *MetricsTracker) IncrementHighValue() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.highValueTrades++
}

// IncrementSignal increments the counter for a specific signal type.
func (m *MetricsTracker) IncrementSignal(signalType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signalsByType[signalType]++
}

// RecordPrice records a price point for a market.
func (m *MetricsTracker) RecordPrice(marketID string, price float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	now := time.Now()
	point := PricePoint{Price: price, Timestamp: now}
	
	// Add to price history
	history := m.priceHistory[marketID]
	history = append(history, point)
	
	// Keep only last 60 minutes
	cutoff := now.Add(-60 * time.Minute)
	validIdx := 0
	for i, p := range history {
		if p.Timestamp.After(cutoff) {
			validIdx = i
			break
		}
	}
	if validIdx > 0 {
		history = history[validIdx:]
	}
	
	m.priceHistory[marketID] = history
}

// UpdateMarketActivity updates activity stats for a market.
func (m *MetricsTracker) UpdateMarketActivity(marketID, question string, price, volume float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	activity, exists := m.marketActivity[marketID]
	if !exists {
		activity = &MarketActivity{
			MarketID:    marketID,
			Question:    question,
			PricePoints: make([]PricePoint, 0, 100),
		}
		m.marketActivity[marketID] = activity
	}
	
	activity.TradeCount++
	activity.Volume += volume
	activity.LastPrice = price
	activity.LastUpdate = time.Now()
	
	// Add price point
	activity.PricePoints = append(activity.PricePoints, PricePoint{
		Price:     price,
		Timestamp: time.Now(),
	})
	
	// Keep only last 60 minutes
	cutoff := time.Now().Add(-60 * time.Minute)
	validIdx := 0
	for i, p := range activity.PricePoints {
		if p.Timestamp.After(cutoff) {
			validIdx = i
			break
		}
	}
	if validIdx > 0 {
		activity.PricePoints = activity.PricePoints[validIdx:]
	}
}

// SetWebSocketStatus sets the WebSocket connection status.
func (m *MetricsTracker) SetWebSocketStatus(status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wsStatus = status
}

// SetRESTLastPoll sets the last REST API poll time.
func (m *MetricsTracker) SetRESTLastPoll(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restLastPoll = t
}

// SetChannelBuffer sets the channel buffer usage.
func (m *MetricsTracker) SetChannelBuffer(used, capacity int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channelBufferUsed = used
	m.channelBufferCap = capacity
}

// Snapshot returns a point-in-time snapshot of metrics.
func (m *MetricsTracker) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Calculate trade rate (trades per second over last 60s)
	tradeRate := 0.0
	if len(m.tradeTimestamps) > 0 {
		oldestTime := m.tradeTimestamps[0]
		duration := time.Since(oldestTime).Seconds()
		if duration > 0 {
			tradeRate = float64(len(m.tradeTimestamps)) / duration
		}
	}
	
	// Copy signals map
	signalsCopy := make(map[string]int64)
	for k, v := range m.signalsByType {
		signalsCopy[k] = v
	}
	
	// Copy market activities
	activitiesCopy := make(map[string]*MarketActivity)
	for k, v := range m.marketActivity {
		activityCopy := *v
		activitiesCopy[k] = &activityCopy
	}
	
	// Calculate top movers
	topMovers := m.calculateTopMovers()
	
	return MetricsSnapshot{
		TradesTotal:       m.tradesTotal,
		HighValueTrades:   m.highValueTrades,
		SignalsByType:     signalsCopy,
		TradeRate:         tradeRate,
		MarketActivities:  activitiesCopy,
		TopMovers:         topMovers,
		Uptime:            time.Since(m.startTime),
		WebSocketStatus:   m.wsStatus,
		RESTAPILastPoll:   m.restLastPoll,
		ChannelBufferUsed: m.channelBufferUsed,
		ChannelBufferCap:  m.channelBufferCap,
	}
}

// calculateTopMovers finds markets with largest price changes.
// Must be called with lock held.
func (m *MetricsTracker) calculateTopMovers() []MoverStats {
	movers := make([]MoverStats, 0, len(m.marketActivity))
	
	for marketID, activity := range m.marketActivity {
		if len(activity.PricePoints) < 2 {
			continue
		}
		
		// Calculate price change over last available period
		firstPrice := activity.PricePoints[0].Price
		lastPrice := activity.PricePoints[len(activity.PricePoints)-1].Price
		
		if firstPrice == 0 {
			continue
		}
		
		priceChange := ((lastPrice - firstPrice) / firstPrice) * 100
		
		movers = append(movers, MoverStats{
			MarketID:     marketID,
			Question:     activity.Question,
			PriceChange:  priceChange,
			Volume:       activity.Volume,
			TradeCount:   activity.TradeCount,
			CurrentPrice: lastPrice,
		})
	}
	
	// Sort by absolute price change (largest first)
	// Note: In production, use sort.Slice with math.Abs comparison
	// For now, return unsorted (UI will handle it)
	
	return movers
}

// Cleanup removes stale data from the tracker.
func (m *MetricsTracker) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	now := time.Now()
	cutoff := now.Add(-60 * time.Minute)
	
	// Clean up market activities with no recent updates
	for id, activity := range m.marketActivity {
		if activity.LastUpdate.Before(cutoff) {
			delete(m.marketActivity, id)
			delete(m.priceHistory, id)
		}
	}
}

