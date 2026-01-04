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

	// Wait for shutdown signal
	go func() {
		sig := <-sigChan
		slog.Info("shutdown_signal_received", "signal", sig.String())
		cancel()
	}()

	slog.Info("engine_ready", "status", "waiting for shutdown signal (Ctrl+C)")

	// Block until context is cancelled
	<-ctx.Done()

	slog.Info("shutdown_complete")
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

