package config

import "time"

// Config represents the main configuration structure
type Config struct {
	Server       ServerConfig        `yaml:"server"`
	Destinations []DestinationConfig `yaml:"destinations"`
}

// ServerConfig represents server configuration
type ServerConfig struct {
	Host         string        `yaml:"host"`
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	Auth         AuthConfig    `yaml:"auth"`
}

// AuthConfig represents authentication configuration
type AuthConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	APIUsername string `yaml:"api_username"`
	APIPassword string `yaml:"api_password"`
}

// DestinationConfig represents a single destination configuration
type DestinationConfig struct {
	Name             string            `yaml:"name"`
	Method           string            `yaml:"method"`
	URL              string            `yaml:"url"`
	Headers          map[string]string `yaml:"headers"`
	Format           string            `yaml:"format"`
	Engine           string            `yaml:"engine"`
	Template         string            `yaml:"template"`
	Transform        string            `yaml:"transform"`
	PostTemplate     string            `yaml:"post_template"`
	SplitAlerts      bool              `yaml:"split_alerts"`
	BatchSize        int               `yaml:"batch_size"`
	ParallelRequests int               `yaml:"parallel_requests"`
	Enabled          bool              `yaml:"enabled"`
}

// GetDestinationByName returns a destination configuration by name (only enabled destinations)
func (c *Config) GetDestinationByName(name string) *DestinationConfig {
	for i := range c.Destinations {
		if c.Destinations[i].Name == name && c.Destinations[i].Enabled {
			return &c.Destinations[i]
		}
	}
	return nil
}

// GetDestinationByNameAny returns a destination configuration by name regardless of enabled status
func (c *Config) GetDestinationByNameAny(name string) *DestinationConfig {
	for i := range c.Destinations {
		if c.Destinations[i].Name == name {
			return &c.Destinations[i]
		}
	}
	return nil
}
