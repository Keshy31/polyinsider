package ui

import (
	"fmt"
	"sort"

	"github.com/polyinsider/engine/internal/metrics"
	"github.com/rivo/tview"
)

// MarketOverviewView displays subscribed markets and their key metrics.
type MarketOverviewView struct {
	table *tview.Table
}

// NewMarketOverviewView creates a new market overview view.
func NewMarketOverviewView() *MarketOverviewView {
	table := tview.NewTable().
		SetBorders(false).
		SetFixed(1, 0)
	
	table.SetTitle(" Market Overview ").SetBorder(true)
	
	// Set header
	headers := []string{"Market", "Trades", "Volume", "Price", "Updated"}
	for col, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetAlign(tview.AlignLeft).
			SetSelectable(false).
			SetExpansion(1)
		table.SetCell(0, col, cell)
	}
	
	return &MarketOverviewView{
		table: table,
	}
}

// Widget returns the tview primitive.
func (v *MarketOverviewView) Widget() tview.Primitive {
	return v.table
}

// Update refreshes the view with new metrics data.
func (v *MarketOverviewView) Update(snapshot metrics.MetricsSnapshot) {
	// Keep header row, clear data rows
	v.table.Clear()
	
	// Re-add header
	headers := []string{"Market", "Trades", "Volume", "Price", "Updated"}
	for col, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
		v.table.SetCell(0, col, cell)
	}
	
	// Sort markets by trade count (most active first)
	markets := make([]*metrics.MarketActivity, 0, len(snapshot.MarketActivities))
	for _, activity := range snapshot.MarketActivities {
		markets = append(markets, activity)
	}
	
	sort.Slice(markets, func(i, j int) bool {
		return markets[i].TradeCount > markets[j].TradeCount
	})
	
	// Show top 10 markets
	limit := 10
	if len(markets) < limit {
		limit = len(markets)
	}
	
	for i, market := range markets[:limit] {
		row := i + 1
		
		// Truncate question
		question := market.Question
		if len(question) > 30 {
			question = question[:27] + "..."
		}
		
		// Format time ago
		timeAgo := formatTimeAgo(market.LastUpdate)
		
		cells := []string{
			question,
			fmt.Sprintf("%d", market.TradeCount),
			fmt.Sprintf("$%.0f", market.Volume),
			fmt.Sprintf("%.3f", market.LastPrice),
			timeAgo,
		}
		
		for col, text := range cells {
			cell := tview.NewTableCell(text).
				SetAlign(tview.AlignLeft).
				SetExpansion(1)
			v.table.SetCell(row, col, cell)
		}
	}
	
	// Update title with total count
	totalMarkets := len(snapshot.MarketActivities)
	v.table.SetTitle(fmt.Sprintf(" Market Overview (%d active) ", totalMarkets))
}

