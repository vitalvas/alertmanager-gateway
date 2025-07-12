package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the main configuration structure
type Config struct {
	Server       ServerConfig       `yaml:"server"`
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
	Path             string            `yaml:"path"`
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
	Retry            RetryConfig       `yaml:"retry"`
}

// RetryConfig represents retry configuration
type RetryConfig struct {
	MaxAttempts int    `yaml:"max_attempts"`
	Backoff     string `yaml:"backoff"`
	PerAlert    bool   `yaml:"per_alert"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Expand environment variables
	expanded := expandEnvVars(string(data))

	var config Config
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	config.setDefaults()

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

// expandEnvVars expands environment variables in the configuration
func expandEnvVars(input string) string {
	// Pattern to match ${VAR} or $VAR
	re := regexp.MustCompile(`\$\{([^}]+)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)
	
	return re.ReplaceAllStringFunc(input, func(match string) string {
		// Remove ${ } or $ from the match
		varName := match
		if strings.HasPrefix(match, "${") && strings.HasSuffix(match, "}") {
			varName = match[2 : len(match)-1]
		} else if strings.HasPrefix(match, "$") {
			varName = match[1:]
		}
		
		// Support default values with ${VAR:-default}
		parts := strings.SplitN(varName, ":-", 2)
		envValue := os.Getenv(parts[0])
		
		if envValue == "" && len(parts) > 1 {
			return parts[1]
		}
		
		return envValue
	})
}

// setDefaults sets default values for configuration
func (c *Config) setDefaults() {
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
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
		
		// Default retry config
		if dest.Retry.MaxAttempts == 0 {
			dest.Retry.MaxAttempts = 3
		}
		if dest.Retry.Backoff == "" {
			dest.Retry.Backoff = "exponential"
		}
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate server config
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	if c.Server.Auth.Enabled {
		if c.Server.Auth.Username == "" || c.Server.Auth.Password == "" {
			return fmt.Errorf("auth enabled but username or password not provided")
		}
	}

	// Validate destinations
	if len(c.Destinations) == 0 {
		return fmt.Errorf("no destinations configured")
	}

	destNames := make(map[string]bool)
	destPaths := make(map[string]bool)

	for i, dest := range c.Destinations {
		if dest.Name == "" {
			return fmt.Errorf("destination %d: name is required", i)
		}
		
		if destNames[dest.Name] {
			return fmt.Errorf("duplicate destination name: %s", dest.Name)
		}
		destNames[dest.Name] = true

		if dest.Path == "" {
			return fmt.Errorf("destination %s: path is required", dest.Name)
		}
		
		if !strings.HasPrefix(dest.Path, "/") {
			return fmt.Errorf("destination %s: path must start with /", dest.Name)
		}
		
		if destPaths[dest.Path] {
			return fmt.Errorf("duplicate destination path: %s", dest.Path)
		}
		destPaths[dest.Path] = true

		if dest.URL == "" {
			return fmt.Errorf("destination %s: url is required", dest.Name)
		}

		validMethods := map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}
		if !validMethods[dest.Method] {
			return fmt.Errorf("destination %s: invalid method %s", dest.Name, dest.Method)
		}

		validFormats := map[string]bool{"json": true, "form": true, "query": true}
		if !validFormats[dest.Format] {
			return fmt.Errorf("destination %s: invalid format %s", dest.Name, dest.Format)
		}

		validEngines := map[string]bool{"go-template": true, "jq": true}
		if !validEngines[dest.Engine] {
			return fmt.Errorf("destination %s: invalid engine %s", dest.Name, dest.Engine)
		}

		// Only validate template/transform if engine is explicitly set or there's content
		if dest.Engine == "go-template" && dest.Template == "" && dest.Transform == "" {
			return fmt.Errorf("destination %s: template is required for go-template engine", dest.Name)
		}

		if dest.Engine == "jq" && dest.Transform == "" && dest.Template == "" {
			return fmt.Errorf("destination %s: transform is required for jq engine", dest.Name)
		}

		validBackoffs := map[string]bool{"exponential": true, "linear": true, "constant": true}
		if !validBackoffs[dest.Retry.Backoff] {
			return fmt.Errorf("destination %s: invalid retry backoff %s", dest.Name, dest.Retry.Backoff)
		}
	}

	return nil
}

// GetDestinationByPath returns a destination configuration by path
func (c *Config) GetDestinationByPath(path string) *DestinationConfig {
	for i := range c.Destinations {
		if c.Destinations[i].Path == path && c.Destinations[i].Enabled {
			return &c.Destinations[i]
		}
	}
	return nil
}

// GetDestinationByName returns a destination configuration by name
func (c *Config) GetDestinationByName(name string) *DestinationConfig {
	for i := range c.Destinations {
		if c.Destinations[i].Name == name && c.Destinations[i].Enabled {
			return &c.Destinations[i]
		}
	}
	return nil
}