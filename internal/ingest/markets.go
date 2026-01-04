// Package ingest provides market data fetching functionality.
package ingest

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const (
	// GammaAPIURL is the Polymarket Gamma API endpoint for market data
	GammaAPIURL = "https://gamma-api.polymarket.com/markets"
	// DefaultMarketLimit is the number of markets to fetch
	DefaultMarketLimit = 50
)

// Market represents a Polymarket market from the Gamma API.
type Market struct {
	ID           string  `json:"id"`
	Question     string  `json:"question"`
	Slug         string  `json:"slug"`
	Active       bool    `json:"active"`
	Closed       bool    `json:"closed"`
	Volume       string  `json:"volume"`
	Liquidity    string  `json:"liquidity"`
	ClobTokenIDs string  `json:"clobTokenIds"` // JSON array as string
	VolumeNum    float64 `json:"volumeNum"`
}

// FetchActiveMarkets fetches active markets from the Polymarket Gamma API.
func FetchActiveMarkets(limit int) ([]Market, error) {
	if limit <= 0 {
		limit = DefaultMarketLimit
	}

	url := fmt.Sprintf("%s?active=true&closed=false&limit=%d", GammaAPIURL, limit)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch markets: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var markets []Market
	if err := json.NewDecoder(resp.Body).Decode(&markets); err != nil {
		return nil, fmt.Errorf("failed to decode markets: %w", err)
	}

	return markets, nil
}

// ExtractTokenIDs extracts all token IDs from a list of markets.
func ExtractTokenIDs(markets []Market) []string {
	var tokenIDs []string
	seen := make(map[string]bool)

	for _, market := range markets {
		if market.ClobTokenIDs == "" {
			continue
		}

		// Parse the JSON array of token IDs
		var ids []string
		if err := json.Unmarshal([]byte(market.ClobTokenIDs), &ids); err != nil {
			slog.Debug("failed to parse token IDs", "market", market.Slug, "error", err)
			continue
		}

		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				tokenIDs = append(tokenIDs, id)
			}
		}
	}

	return tokenIDs
}

// GetActiveTokenIDs fetches active markets and returns their token IDs.
func GetActiveTokenIDs(limit int) ([]string, error) {
	markets, err := FetchActiveMarkets(limit)
	if err != nil {
		return nil, err
	}

	tokenIDs := ExtractTokenIDs(markets)
	slog.Info("fetched_active_markets",
		"market_count", len(markets),
		"token_count", len(tokenIDs),
	)

	return tokenIDs, nil
}

