package ui

import (
	"fmt"

	"github.com/polyinsider/engine/internal/store"
	"github.com/rivo/tview"
)

// LiveTradesView displays a scrolling feed of incoming trades.
type LiveTradesView struct {
	table  *tview.Table
	trades []store.Trade
	maxRows int
}

// NewLiveTradesView creates a new live trades view.
func NewLiveTradesView() *LiveTradesView {
	table := tview.NewTable().
		SetBorders(false).
		SetFixed(1, 0)
	
	table.SetTitle(" Live Trades ").SetBorder(true)
	
	// Set header
	headers := []string{"Time", "Market", "Side", "Price", "Value", "Maker"}
	for col, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
		table.SetCell(0, col, cell)
	}
	
	return &LiveTradesView{
		table:   table,
		trades:  make([]store.Trade, 0, 100),
		maxRows: 100,
	}
}

// Widget returns the tview primitive.
func (v *LiveTradesView) Widget() tview.Primitive {
	return v.table
}

// AddTrade adds a new trade to the view.
func (v *LiveTradesView) AddTrade(trade store.Trade) {
	// Add to front of ring buffer
	v.trades = append([]store.Trade{trade}, v.trades...)
	
	// Trim to max rows
	if len(v.trades) > v.maxRows {
		v.trades = v.trades[:v.maxRows]
	}
	
	// Update display
	v.updateTable()
}

// Refresh redraws the table.
func (v *LiveTradesView) Refresh() {
	v.updateTable()
}

// updateTable updates the table with current trades.
func (v *LiveTradesView) updateTable() {
	// Clear table (keep header)
	v.table.Clear()
	
	// Re-add header
	headers := []string{"Time", "Market", "Side", "Price", "Value", "Maker"}
	for col, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
		v.table.SetCell(0, col, cell)
	}
	
	// Add trades
	for i, trade := range v.trades {
		row := i + 1
		
		// Format time
		timeStr := trade.Timestamp.Format("15:04:05")
		
		// Truncate market
		market := trade.MarketID
		if len(market) > 16 {
			market = market[:8] + "..." + market[len(market)-4:]
		}
		
		// Truncate maker
		maker := truncateAddress(trade.MakerAddress)
		if maker == "" {
			maker = "unknown"
		}
		
		// Format side
		side := trade.Side
		if side == "" {
			side = "?"
		}
		
		cells := []string{
			timeStr,
			market,
			side,
			fmt.Sprintf("%.3f", trade.Price),
			fmt.Sprintf("$%.0f", trade.ValueUSD),
			maker,
		}
		
		for col, text := range cells {
			cell := tview.NewTableCell(text).
				SetAlign(tview.AlignLeft)
			v.table.SetCell(row, col, cell)
		}
	}
	
	// Update title with count
	v.table.SetTitle(fmt.Sprintf(" Live Trades (%d) ", len(v.trades)))
}

