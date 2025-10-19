package config

import (
	"fmt"
	"time"

	"github.com/vitalvas/gokit/xconfig"
)

// LoadConfig loads configuration from a YAML file using xconfig
func LoadConfig(path string) (*Config, error) {
	var config Config

	// Load configuration using xconfig with defaults
	defaultConfig := Config{}
	defaultConfig.setDefaults()

	err := xconfig.Load(&config,
		xconfig.WithDefault(defaultConfig),
		xconfig.WithFiles(path),
		xconfig.WithEnv("GATEWAY"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Set any remaining defaults that might not have been set
	config.setDefaults()

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

// setDefaults sets default values for configuration
func (c *Config) setDefaults() {
	// Address defaults to :8080 for dual stack (IPv4 and IPv6)
	if c.Server.Address == "" {
		c.Server.Address = ":8080"
	}
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = 30 * time.Second
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = 30 * time.Second
	}

	for i := range c.Destinations {
		dest := &c.Destinations[i]

		// Default to enabled
		if !dest.Enabled && dest.Name != "" {
			dest.Enabled = true
		}

		// Default HTTP method
		if dest.Method == "" {
			dest.Method = "POST"
		}

		// Default format
		if dest.Format == "" {
			dest.Format = "json"
		}

		// Default engine
		if dest.Engine == "" {
			if dest.Transform != "" {
				dest.Engine = "jq"
			} else {
				dest.Engine = "go-template"
			}
		}

		// Default batch size for split alerts
		if dest.SplitAlerts && dest.BatchSize == 0 {
			dest.BatchSize = 1
		}

		// Default parallel requests
		if dest.ParallelRequests == 0 {
			dest.ParallelRequests = 1
		}
	}
}
