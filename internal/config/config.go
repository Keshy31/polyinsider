// Package config handles loading and validating configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all configuration values for the Polyinsider engine.
type Config struct {
	// Polymarket WebSocket
	PolymarketWSURL string

	// Polymarket REST API
	PolymarketRESTURL string
	TradePollInterval time.Duration

	// Blockchain RPC
	AlchemyAPIKey  string
	AlchemyURL     string
	FallbackRPCURL string

	// Detection Thresholds
	MinValueUSD      float64
	WhaleValueUSD    float64
	FreshWalletNonce int
	BurstCount       int
	BurstWindow      time.Duration

	// Alerting
	DiscordWebhookURL  string
	AlertBatchDuration time.Duration
	AlertCooldown      time.Duration

	// Database
	DBPath string

	// Workers
	WorkerCount int

	// Metrics
	PrometheusPort int

	// UI
	EnableTUI     bool
	UIRefreshRate time.Duration

	// Logging
	LogLevel string
}

// Load reads configuration from environment variables with fallback to .env file.
// Priority order: Environment variables > .env file > hardcoded defaults
func Load() (*Config, error) {
	// Attempt to load .env file (ignore error if not found)
	_ = godotenv.Load()

	cfg := &Config{
		// Polymarket
		PolymarketWSURL:   getEnv("POLYMARKET_WS_URL", "wss://ws-subscriptions-clob.polymarket.com/ws/"),
		PolymarketRESTURL: getEnv("POLYMARKET_REST_URL", "https://clob.polymarket.com"),
		TradePollInterval: time.Duration(getEnvInt("TRADE_POLL_INTERVAL_SECONDS", 3)) * time.Second,

		// RPC
		AlchemyAPIKey:  getEnv("ALCHEMY_API_KEY", ""),
		AlchemyURL:     getEnv("ALCHEMY_URL", "https://polygon-mainnet.g.alchemy.com/v2/"),
		FallbackRPCURL: getEnv("FALLBACK_RPC_URL", "https://polygon-rpc.com"),

		// Thresholds
		MinValueUSD:      getEnvFloat("MIN_VALUE_USD", 2000),
		WhaleValueUSD:    getEnvFloat("WHALE_VALUE_USD", 50000),
		FreshWalletNonce: getEnvInt("FRESH_WALLET_NONCE", 5),
		BurstCount:       getEnvInt("BURST_COUNT", 3),
		BurstWindow:      time.Duration(getEnvInt("BURST_WINDOW_SECONDS", 60)) * time.Second,

		// Alerting
		DiscordWebhookURL:  getEnv("DISCORD_WEBHOOK_URL", ""),
		AlertBatchDuration: time.Duration(getEnvInt("ALERT_BATCH_SECONDS", 30)) * time.Second,
		AlertCooldown:      time.Duration(getEnvInt("ALERT_COOLDOWN_MINUTES", 60)) * time.Minute,

		// Database
		DBPath: getEnv("DB_PATH", "./data/trades.db"),

		// Workers
		WorkerCount: getEnvInt("WORKER_COUNT", 5),

		// Metrics
		PrometheusPort: getEnvInt("PROMETHEUS_PORT", 9090),

		// UI
		EnableTUI:     getEnvBool("ENABLE_TUI", true),
		UIRefreshRate: time.Duration(getEnvInt("UI_REFRESH_MS", 500)) * time.Millisecond,

		// Logging
		LogLevel: getEnv("LOG_LEVEL", "INFO"),
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks that required configuration values are set and valid.
func (c *Config) Validate() error {
	if c.PolymarketWSURL == "" {
		return fmt.Errorf("POLYMARKET_WS_URL is required")
	}

	if c.MinValueUSD <= 0 {
		return fmt.Errorf("MIN_VALUE_USD must be positive")
	}

	if c.WhaleValueUSD <= 0 {
		return fmt.Errorf("WHALE_VALUE_USD must be positive")
	}

	if c.WorkerCount < 1 {
		return fmt.Errorf("WORKER_COUNT must be at least 1")
	}

	if c.PrometheusPort < 1 || c.PrometheusPort > 65535 {
		return fmt.Errorf("PROMETHEUS_PORT must be between 1 and 65535")
	}

	return nil
}

// MaskedAlchemyKey returns the API key with most characters hidden for logging.
func (c *Config) MaskedAlchemyKey() string {
	return maskSecret(c.AlchemyAPIKey)
}

// MaskedDiscordWebhook returns the webhook URL with most characters hidden for logging.
func (c *Config) MaskedDiscordWebhook() string {
	return maskSecret(c.DiscordWebhookURL)
}

// maskSecret hides all but the first and last 4 characters of a secret.
func maskSecret(s string) string {
	if len(s) <= 8 {
		if len(s) == 0 {
			return "(not set)"
		}
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

// getEnv retrieves an environment variable or returns a default value.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt retrieves an environment variable as an integer or returns a default.
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// getEnvFloat retrieves an environment variable as a float64 or returns a default.
func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
	}
	return defaultValue
}

// getEnvBool retrieves an environment variable as a boolean or returns a default.
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

