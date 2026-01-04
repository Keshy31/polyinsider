// Package main is the entry point for the Polyinsider surveillance engine.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/polyinsider/engine/internal/config"
	"github.com/polyinsider/engine/internal/detector"
	"github.com/polyinsider/engine/internal/ingest"
	"github.com/polyinsider/engine/internal/metrics"
	"github.com/polyinsider/engine/internal/store"
	"github.com/polyinsider/engine/internal/ui"
)

const (
	// TradeChannelBuffer is the size of the buffered trade channel
	TradeChannelBuffer = 1000
	// SuspectChannelBuffer is the size of the buffered suspect channel
	SuspectChannelBuffer = 100
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize structured logger
	logger := setupLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	// Log startup with configuration (secrets masked)
	slog.Info("polyinsider starting",
		"version", "1.0.0",
	)

	slog.Info("config_loaded",
		"polymarket_ws_url", cfg.PolymarketWSURL,
		"polymarket_rest_url", cfg.PolymarketRESTURL,
		"enable_tui", cfg.EnableTUI,
		"alchemy_key", cfg.MaskedAlchemyKey(),
		"discord_webhook", cfg.MaskedDiscordWebhook(),
		"min_value_usd", cfg.MinValueUSD,
		"whale_value_usd", cfg.WhaleValueUSD,
		"fresh_wallet_nonce", cfg.FreshWalletNonce,
		"burst_count", cfg.BurstCount,
		"burst_window", cfg.BurstWindow,
		"worker_count", cfg.WorkerCount,
		"db_path", cfg.DBPath,
		"prometheus_port", cfg.PrometheusPort,
	)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create channels
	tradeChan := make(chan store.Trade, TradeChannelBuffer)
	suspectChan := make(chan store.Suspect, SuspectChannelBuffer)

	// Initialize metrics tracker
	tracker := metrics.NewMetricsTracker()
	
	// Start periodic cleanup
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tracker.Cleanup()
			}
		}
	}()

	// Initialize detector
	detect := detector.NewDetector(cfg)

	// Fetch active market token IDs
	slog.Info("fetching_active_markets")
	markets, err := ingest.FetchActiveMarkets(100)
	if err != nil {
		slog.Warn("failed to fetch active markets, will subscribe to empty set", "error", err)
		markets = []ingest.Market{}
	}
	tokenIDs := ingest.ExtractTokenIDs(markets)

	// Initialize market activity in tracker
	for _, market := range markets {
		tracker.UpdateMarketActivity(market.ID, market.Question, 0, 0)
	}

	// Start WebSocket listener with active market tokens
	listener := ingest.NewListener(cfg.PolymarketWSURL, tradeChan)
	listener.SetAssetIDs(tokenIDs)
	listener.Start(ctx)
	tracker.SetWebSocketStatus("connected")

	// Start REST API poller (optional - will fail gracefully if endpoint doesn't exist)
	if cfg.PolymarketRESTURL != "" {
		poller := ingest.NewTradesPoller(cfg.PolymarketRESTURL, cfg.TradePollInterval, tradeChan)
		go poller.Start(ctx)
		slog.Info("rest_poller_started", "url", cfg.PolymarketRESTURL, "interval", cfg.TradePollInterval)
	}

	// Start worker pool to process trades
	for i := 0; i < cfg.WorkerCount; i++ {
		go worker(ctx, i, tradeChan, suspectChan, detect, tracker, cfg)
	}

	slog.Info("engine_started", 
		"status", "listening for trades", 
		"subscribed_tokens", len(tokenIDs),
		"workers", cfg.WorkerCount,
		"tui_enabled", cfg.EnableTUI,
	)

	// Start TUI or run in background mode
	if cfg.EnableTUI {
		// TUI mode (blocking)
		slog.Info("starting_tui")
		app := ui.NewApp(tradeChan, suspectChan, tracker)
		
		// Start TUI in goroutine so we can still handle signals
		go func() {
			if err := app.Run(); err != nil {
				slog.Error("tui_error", "error", err)
				cancel()
			}
		}()
		
		// Wait for shutdown signal or context cancellation
		select {
		case sig := <-sigChan:
			slog.Info("shutdown_signal_received", "signal", sig.String())
			app.Stop()
		case <-ctx.Done():
			app.Stop()
		}
	} else {
		// Background mode - just wait for signal
		sig := <-sigChan
		slog.Info("shutdown_signal_received", "signal", sig.String())
	}
	
	cancel()

	// Graceful shutdown
	slog.Info("shutting_down", "status", "stopping listener")
	listener.Stop()

	// Drain remaining trades
	drainTrades(tradeChan)

	slog.Info("shutdown_complete")
}

// worker processes trades, detects signals, and updates metrics.
func worker(ctx context.Context, id int, tradeChan <-chan store.Trade, 
	suspectChan chan<- store.Suspect, detect *detector.Detector, 
	tracker *metrics.MetricsTracker, cfg *config.Config) {
	
	slog.Debug("worker_started", "id", id)
	defer slog.Debug("worker_stopped", "id", id)
	
	for {
		select {
		case <-ctx.Done():
			return
		case trade, ok := <-tradeChan:
			if !ok {
				return
			}
			
			// Update metrics
			tracker.IncrementTrades()
			tracker.RecordPrice(trade.MarketID, trade.Price)
			
			// Update market activity
			tracker.UpdateMarketActivity(trade.MarketID, "", trade.Price, trade.ValueUSD)
			
			// Update channel buffer metrics
			tracker.SetChannelBuffer(len(tradeChan), cap(tradeChan))
			
			// Track high-value trades
			if trade.ValueUSD >= cfg.MinValueUSD {
				tracker.IncrementHighValue()
			}
			
			// Detect signals (no nonce yet, pass -1)
			suspects := detect.Detect(trade, -1)
			for _, suspect := range suspects {
				tracker.IncrementSignal(suspect.SignalType)
				
				// Send to suspect channel
				select {
				case suspectChan <- suspect:
					slog.Debug("signal_detected", 
						"type", suspect.SignalType, 
						"market", truncateID(suspect.Trade.MarketID),
						"value_usd", suspect.Trade.ValueUSD,
					)
				default:
					slog.Warn("suspect_channel_full", "signal_type", suspect.SignalType)
				}
			}
		}
	}
}

// drainTrades processes remaining trades in the channel during shutdown.
func drainTrades(tradeChan <-chan store.Trade) {
	timeout := time.After(5 * time.Second)
	drained := 0

	for {
		select {
		case <-tradeChan:
			drained++
		case <-timeout:
			if drained > 0 {
				slog.Info("trades_drained", "count", drained)
			}
			return
		default:
			if drained > 0 {
				slog.Info("trades_drained", "count", drained)
			}
			return
		}
	}
}

// truncateID shortens an ID for logging.
func truncateID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:6] + "..." + id[len(id)-4:]
}

// setupLogger creates a structured logger with the specified level.
// Format: 2025-01-04 14:32:01 [INFO]  message key=value
func setupLogger(levelStr string) *slog.Logger {
	var level slog.Level
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
		level = slog.LevelInfo
	case "WARN", "WARNING":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Format time as specified in the spec
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.StringValue(t.Format("2006-01-02 15:04:05"))
				}
			}
			return a
		},
	}

	handler := slog.NewTextHandler(os.Stdout, opts)
	return slog.New(handler)
}
