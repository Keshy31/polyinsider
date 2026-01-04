package ui

import (
	"fmt"
	"math"
	"sort"

	"github.com/gdamore/tcell/v2"
	"github.com/polyinsider/engine/internal/metrics"
	"github.com/rivo/tview"
)

// TopMoversView displays markets with the highest activity and price changes.
type TopMoversView struct {
	table *tview.Table
}

// NewTopMoversView creates a new top movers view.
func NewTopMoversView() *TopMoversView {
	table := tview.NewTable().
		SetBorders(false).
		SetFixed(1, 0)
	
	table.SetTitle(" Top Movers ").SetBorder(true)
	
	// Set header
	headers := []string{"Market", "Change", "Trades", "Volume"}
	for col, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
		table.SetCell(0, col, cell)
	}
	
	return &TopMoversView{
		table: table,
	}
}

// Widget returns the tview primitive.
func (v *TopMoversView) Widget() tview.Primitive {
	return v.table
}

// Update refreshes the top movers display.
func (v *TopMoversView) Update(snapshot metrics.MetricsSnapshot) {
	// Clear table (keep header)
	v.table.Clear()
	
	// Re-add header
	headers := []string{"Market", "Change", "Trades", "Volume"}
	for col, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
		v.table.SetCell(0, col, cell)
	}
	
	// Get movers and sort by absolute price change
	movers := snapshot.TopMovers
	sort.Slice(movers, func(i, j int) bool {
		return math.Abs(movers[i].PriceChange) > math.Abs(movers[j].PriceChange)
	})
	
	// Show top 10
	limit := 10
	if len(movers) < limit {
		limit = len(movers)
	}
	
	if limit == 0 {
		// No data yet
		cell := tview.NewTableCell("No data yet...").
			SetAlign(tview.AlignCenter).
			SetExpansion(1)
		v.table.SetCell(1, 0, cell)
		return
	}
	
	for i, mover := range movers[:limit] {
		row := i + 1
		
		// Truncate question
		question := mover.Question
		if len(question) > 25 {
			question = question[:22] + "..."
		}
		
		// Format price change with color
		changeStr := fmt.Sprintf("%+.2f%%", mover.PriceChange)
		changeColor := tcell.ColorWhite
		if mover.PriceChange > 0 {
			changeColor = tcell.ColorGreen
		} else if mover.PriceChange < 0 {
			changeColor = tcell.ColorRed
		}
		
		// Market name
		cell := tview.NewTableCell(question).SetAlign(tview.AlignLeft)
		v.table.SetCell(row, 0, cell)
		
		// Price change
		cell = tview.NewTableCell(changeStr).
			SetAlign(tview.AlignRight).
			SetTextColor(changeColor)
		v.table.SetCell(row, 1, cell)
		
		// Trade count
		cell = tview.NewTableCell(fmt.Sprintf("%d", mover.TradeCount)).
			SetAlign(tview.AlignRight)
		v.table.SetCell(row, 2, cell)
		
		// Volume
		cell = tview.NewTableCell(fmt.Sprintf("$%.0f", mover.Volume)).
			SetAlign(tview.AlignRight)
		v.table.SetCell(row, 3, cell)
	}
}

