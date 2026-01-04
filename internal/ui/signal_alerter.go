package ui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/polyinsider/engine/internal/store"
	"github.com/rivo/tview"
)

// SignalAlerterView displays detected trading signals.
type SignalAlerterView struct {
	list     *tview.List
	suspects []store.Suspect
	maxItems int
}

// NewSignalAlerterView creates a new signal alerter view.
func NewSignalAlerterView() *SignalAlerterView {
	list := tview.NewList().
		ShowSecondaryText(true)
	
	list.SetTitle(" 🚨 Signal Alerts ").SetBorder(true)
	list.SetMainTextColor(tcell.ColorWhite)
	
	return &SignalAlerterView{
		list:     list,
		suspects: make([]store.Suspect, 0, 50),
		maxItems: 50,
	}
}

// Widget returns the tview primitive.
func (v *SignalAlerterView) Widget() tview.Primitive {
	return v.list
}

// AddSuspect adds a new suspect to the alerts list.
func (v *SignalAlerterView) AddSuspect(suspect store.Suspect) {
	// Add to front of list
	v.suspects = append([]store.Suspect{suspect}, v.suspects...)
	
	// Trim to max items
	if len(v.suspects) > v.maxItems {
		v.suspects = v.suspects[:v.maxItems]
	}
	
	// Rebuild list
	v.rebuildList()
}

// Refresh redraws the list.
func (v *SignalAlerterView) Refresh() {
	v.rebuildList()
}

// rebuildList rebuilds the entire list from suspects.
func (v *SignalAlerterView) rebuildList() {
	v.list.Clear()
	
	if len(v.suspects) == 0 {
		v.list.AddItem("No signals detected yet", "", 0, nil)
		return
	}
	
	for _, suspect := range v.suspects {
		mainText, secondaryText, _ := v.formatSuspect(suspect)
		
		// Add list item (color formatting done via text markup)
		v.list.AddItem(mainText, secondaryText, 0, nil)
	}
	
	// Update title with count
	v.list.SetTitle(fmt.Sprintf(" 🚨 Signal Alerts (%d) ", len(v.suspects)))
}

// formatSuspect formats a suspect for display.
func (v *SignalAlerterView) formatSuspect(suspect store.Suspect) (string, string, tcell.Color) {
	// Determine color and icon based on signal type
	var icon string
	var color tcell.Color
	
	switch suspect.SignalType {
	case store.SignalFreshInsider:
		icon = "🔴"
		color = tcell.ColorRed
	case store.SignalWhale:
		icon = "🐋"
		color = tcell.ColorBlue
	case store.SignalPanicBurst:
		icon = "⚡"
		color = tcell.ColorYellow
	case store.SignalPriceShock:
		icon = "📈"
		color = tcell.ColorGreen
	default:
		icon = "❓"
		color = tcell.ColorWhite
	}
	
	// Format time
	timeStr := suspect.Trade.Timestamp.Format("15:04:05")
	
	// Truncate wallet address
	wallet := truncateAddress(suspect.Trade.MakerAddress)
	
	// Truncate market
	market := suspect.Trade.MarketID
	if len(market) > 20 {
		market = market[:8] + "..." + market[len(market)-8:]
	}
	
	// Main text: Time + Icon + Signal Type
	mainText := fmt.Sprintf("%s %s %s", timeStr, icon, suspect.SignalType)
	
	// Secondary text: Wallet, Value, Market
	secondaryText := fmt.Sprintf("Wallet: %s | $%.2f | %s", 
		wallet, suspect.Trade.ValueUSD, market)
	
	// Add nonce if available
	if suspect.Nonce >= 0 {
		secondaryText += fmt.Sprintf(" | Nonce: %d", suspect.Nonce)
	}
	
	// Add meta info if available (e.g., price change for PRICE_SHOCK)
	if len(suspect.Meta) > 0 {
		if pctChange, ok := suspect.Meta["pct_change"].(float64); ok {
			secondaryText += fmt.Sprintf(" | Δ%.2f%%", pctChange*100)
		}
	}
	
	return mainText, secondaryText, color
}

// truncateAddress truncates a wallet address for display.
func truncateAddress(addr string) string {
	if len(addr) <= 12 {
		return addr
	}
	return addr[:6] + "..." + addr[len(addr)-4:]
}

