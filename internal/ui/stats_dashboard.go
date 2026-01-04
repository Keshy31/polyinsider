package ui

import (
	"fmt"
	"time"

	"github.com/polyinsider/engine/internal/metrics"
	"github.com/rivo/tview"
)

// StatsDashboardView displays system health and performance metrics.
type StatsDashboardView struct {
	textView *tview.TextView
}

// NewStatsDashboardView creates a new stats dashboard view.
func NewStatsDashboardView() *StatsDashboardView {
	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false)
	
	textView.SetTitle(" Stats Dashboard ").SetBorder(true)
	
	return &StatsDashboardView{
		textView: textView,
	}
}

// Widget returns the tview primitive.
func (v *StatsDashboardView) Widget() tview.Primitive {
	return v.textView
}

// Update refreshes the stats display.
func (v *StatsDashboardView) Update(snapshot metrics.MetricsSnapshot) {
	v.textView.Clear()
	
	// Format uptime
	uptime := formatDuration(snapshot.Uptime)
	
	// Format WebSocket status
	wsStatus := snapshot.WebSocketStatus
	wsColor := "red"
	if wsStatus == "connected" {
		wsColor = "green"
	}
	
	// Format REST API status
	restStatus := "never"
	if !snapshot.RESTAPILastPoll.IsZero() {
		restStatus = formatTimeAgo(snapshot.RESTAPILastPoll)
	}
	
	// Calculate buffer usage percentage
	bufferPct := 0.0
	if snapshot.ChannelBufferCap > 0 {
		bufferPct = (float64(snapshot.ChannelBufferUsed) / float64(snapshot.ChannelBufferCap)) * 100
	}
	
	// Build stats text
	text := fmt.Sprintf(`[yellow]System Status[-]
Uptime: %s
WebSocket: [%s]%s[-]
REST API: %s

[yellow]Trade Stats[-]
Total Trades: %d
High Value: %d
Rate: %.2f trades/sec

[yellow]Signals Detected[-]
Fresh Insider: %d
Whale: %d
Panic Burst: %d
Price Shock: %d

[yellow]Performance[-]
Channel Buffer: %d/%d (%.1f%%)
`,
		uptime,
		wsColor, wsStatus,
		restStatus,
		snapshot.TradesTotal,
		snapshot.HighValueTrades,
		snapshot.TradeRate,
		snapshot.SignalsByType["FRESH_INSIDER"],
		snapshot.SignalsByType["WHALE"],
		snapshot.SignalsByType["PANIC_BURST"],
		snapshot.SignalsByType["PRICE_SHOCK"],
		snapshot.ChannelBufferUsed,
		snapshot.ChannelBufferCap,
		bufferPct,
	)
	
	fmt.Fprint(v.textView, text)
}

// formatDuration formats a duration in human-readable form.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

// formatTimeAgo formats a time as "X ago".
func formatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	
	elapsed := time.Since(t)
	
	if elapsed < time.Minute {
		return fmt.Sprintf("%.0fs ago", elapsed.Seconds())
	}
	if elapsed < time.Hour {
		return fmt.Sprintf("%.0fm ago", elapsed.Minutes())
	}
	if elapsed < 24*time.Hour {
		return fmt.Sprintf("%.0fh ago", elapsed.Hours())
	}
	return fmt.Sprintf("%.0fd ago", elapsed.Hours()/24)
}

