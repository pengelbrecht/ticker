package verify

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config holds verification configuration loaded from .ticker/config.json.
type Config struct {
	// Enabled controls whether verification runs (default true).
	// Set to false to completely skip verification.
	Enabled *bool `json:"enabled,omitempty"`
}

// IsEnabled returns whether verification is enabled (default true).
func (c *Config) IsEnabled() bool {
	if c == nil || c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// TickerConfig is the root config structure for .ticker/config.json.
type TickerConfig struct {
	Verification *Config `json:"verification,omitempty"`
}

// LoadConfig loads configuration from .ticker/config.json in the given directory.
// Returns nil config (not error) if file doesn't exist.
// Returns error only for malformed JSON.
func LoadConfig(dir string) (*Config, error) {
	configPath := filepath.Join(dir, ".ticker", "config.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Missing config file = verification enabled by default
			return nil, nil
		}
		return nil, err
	}

	var tickerConfig TickerConfig
	if err := json.Unmarshal(data, &tickerConfig); err != nil {
		return nil, err
	}

	return tickerConfig.Verification, nil
}
