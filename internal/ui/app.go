// Package ui provides terminal user interface components.
package ui

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/polyinsider/engine/internal/metrics"
	"github.com/polyinsider/engine/internal/store"
	"github.com/rivo/tview"
)

// App is the main TUI application.
type App struct {
	app              *tview.Application
	pages            *tview.Pages
	layout           *tview.Flex
	
	// Views
	marketOverview   *MarketOverviewView
	signalAlerter    *SignalAlerterView
	liveTrades       *LiveTradesView
	statsDashboard   *StatsDashboardView
	topMovers        *TopMoversView
	
	// Data channels
	tradeChan        <-chan store.Trade
	suspectChan      <-chan store.Suspect
	metricsTracker   *metrics.MetricsTracker
	
	// State
	mu               sync.Mutex
	ctx              context.Context
	cancel           context.CancelFunc
}

// NewApp creates a new TUI application.
func NewApp(tradeChan <-chan store.Trade, suspectChan <-chan store.Suspect, tracker *metrics.MetricsTracker) *App {
	ctx, cancel := context.WithCancel(context.Background())
	
	app := &App{
		app:            tview.NewApplication(),
		tradeChan:      tradeChan,
		suspectChan:    suspectChan,
		metricsTracker: tracker,
		ctx:            ctx,
		cancel:         cancel,
	}
	
	// Initialize views
	app.marketOverview = NewMarketOverviewView()
	app.signalAlerter = NewSignalAlerterView()
	app.liveTrades = NewLiveTradesView()
	app.statsDashboard = NewStatsDashboardView()
	app.topMovers = NewTopMoversView()
	
	// Setup layout
	app.setupLayout()
	
	// Setup keyboard shortcuts
	app.setupKeyboard()
	
	return app
}

// setupLayout creates the 5-panel layout.
func (a *App) setupLayout() {
	// Top row: Market Overview (left) | Signal Alerter (right)
	topRow := tview.NewFlex().
		AddItem(a.marketOverview.Widget(), 0, 1, false).
		AddItem(a.signalAlerter.Widget(), 0, 2, false)
	
	// Middle row: Live Trades (full width)
	middleRow := a.liveTrades.Widget()
	
	// Bottom row: Stats Dashboard (left) | Top Movers (right)
	bottomRow := tview.NewFlex().
		AddItem(a.statsDashboard.Widget(), 0, 1, false).
		AddItem(a.topMovers.Widget(), 0, 1, false)
	
	// Main vertical layout
	a.layout = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(topRow, 0, 2, false).
		AddItem(middleRow, 0, 3, false).
		AddItem(bottomRow, 0, 2, false)
	
	a.app.SetRoot(a.layout, true)
}

// setupKeyboard configures keyboard shortcuts.
func (a *App) setupKeyboard() {
	a.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyCtrlC:
			// Quit application
			a.Stop()
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case 'q', 'Q':
				// Quit application
				a.Stop()
				return nil
			case 'r', 'R':
				// Refresh all views
				a.refresh()
				return nil
			}
		}
		return event
	})
}

// Run starts the TUI application (blocking).
func (a *App) Run() error {
	// Start data processing goroutines
	go a.processTrades()
	go a.processSuspects()
	go a.updateLoop()
	
	// Run the TUI (blocking)
	if err := a.app.Run(); err != nil {
		return fmt.Errorf("app run failed: %w", err)
	}
	
	return nil
}

// Stop gracefully stops the application.
func (a *App) Stop() {
	a.cancel()
	a.app.Stop()
}

// processTrades reads from the trade channel and updates views.
func (a *App) processTrades() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case trade, ok := <-a.tradeChan:
			if !ok {
				return
			}
			
			// Update views with new trade
			a.app.QueueUpdateDraw(func() {
				a.liveTrades.AddTrade(trade)
			})
		}
	}
}

// processSuspects reads from the suspect channel and updates the signal alerter.
func (a *App) processSuspects() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case suspect, ok := <-a.suspectChan:
			if !ok {
				return
			}
			
			// Update signal alerter with new suspect
			a.app.QueueUpdateDraw(func() {
				a.signalAlerter.AddSuspect(suspect)
			})
		}
	}
}

// updateLoop periodically refreshes views with metrics data.
func (a *App) updateLoop() {
	ticker := time.NewTicker(500 * time.Millisecond) // 500ms refresh rate
	defer ticker.Stop()
	
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			snapshot := a.metricsTracker.Snapshot()
			
			a.app.QueueUpdateDraw(func() {
				a.statsDashboard.Update(snapshot)
				a.topMovers.Update(snapshot)
				a.marketOverview.Update(snapshot)
			})
		}
	}
}

// refresh manually refreshes all views.
func (a *App) refresh() {
	snapshot := a.metricsTracker.Snapshot()
	
	a.app.QueueUpdateDraw(func() {
		a.marketOverview.Update(snapshot)
		a.signalAlerter.Refresh()
		a.liveTrades.Refresh()
		a.statsDashboard.Update(snapshot)
		a.topMovers.Update(snapshot)
	})
}

