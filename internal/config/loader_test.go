package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name             string
		configContent    string
		envVars          map[string]string
		expectedHost     string
		expectedPort     int
		expectedPassword string
		expectedURL      string
		additionalChecks func(t *testing.T, cfg *Config)
	}{
		{
			name: "basic configuration",
			configContent: `
server:
  host: "127.0.0.1"
  port: 8090
  read_timeout: 45s
  write_timeout: 45s
  auth:
    enabled: true
    username: "test"
    password: "secret"

destinations:
  - name: "test-dest"
    method: "POST"
    url: "https://example.com/webhook"
    format: "json"
    engine: "jq"
    transform: |
      {
        message: .message
      }
`,
			expectedHost:     "127.0.0.1",
			expectedPort:     8090,
			expectedPassword: "secret",
			expectedURL:      "https://example.com/webhook",
			additionalChecks: func(t *testing.T, cfg *Config) {
				assert.Equal(t, 45*time.Second, cfg.Server.ReadTimeout)
				assert.Equal(t, 45*time.Second, cfg.Server.WriteTimeout)
				assert.True(t, cfg.Server.Auth.Enabled)
				assert.Equal(t, "test", cfg.Server.Auth.Username)

				require.Len(t, cfg.Destinations, 1)
				dest := cfg.Destinations[0]
				assert.Equal(t, "test-dest", dest.Name)
				assert.Equal(t, "POST", dest.Method)
				assert.Equal(t, "json", dest.Format)
				assert.Equal(t, "jq", dest.Engine)
				assert.Contains(t, dest.Transform, "message: .message")
				assert.True(t, dest.Enabled)
			},
		},
		{
			name: "with environment variables",
			configContent: `
server:
  host: "localhost"  # Will be overridden by env var
  port: 8080         # Will be overridden by env var
  auth:
    enabled: true
    username: "admin"
    password: "default-pass"  # Will be overridden by env var

destinations:
  - name: "env-test"
    url: "https://example.com/hook"
    format: "json"
    template: '{"test": "data"}'
`,
			envVars: map[string]string{
				"GATEWAY_SERVER_HOST":          "192.168.1.1",
				"GATEWAY_SERVER_PORT":          "9090",
				"GATEWAY_SERVER_AUTH_PASSWORD": "env-secret",
			},
			expectedHost:     "192.168.1.1",
			expectedPort:     9090,
			expectedPassword: "env-secret",
			expectedURL:      "https://example.com/hook",
			additionalChecks: func(t *testing.T, cfg *Config) {
				require.Len(t, cfg.Destinations, 1)
				assert.Equal(t, "env-test", cfg.Destinations[0].Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables if provided
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer func() {
				for key := range tt.envVars {
					os.Unsetenv(key)
				}
			}()

			// Create temporary config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")

			err := os.WriteFile(configPath, []byte(tt.configContent), 0644)
			require.NoError(t, err)

			// Load config
			cfg, err := LoadConfig(configPath)
			require.NoError(t, err)
			require.NotNil(t, cfg)

			// Common assertions
			assert.Equal(t, tt.expectedHost, cfg.Server.Host)
			assert.Equal(t, tt.expectedPort, cfg.Server.Port)
			assert.Equal(t, tt.expectedPassword, cfg.Server.Auth.Password)
			assert.Equal(t, tt.expectedURL, cfg.Destinations[0].URL)

			// Additional test-specific checks
			if tt.additionalChecks != nil {
				tt.additionalChecks(t, cfg)
			}
		})
	}
}

func TestConfig_SetDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	minimalConfig := `
destinations:
  - name: "minimal"
    url: "https://example.com"
    template: '{"message": "{{ .Status }}"}'
`

	err := os.WriteFile(configPath, []byte(minimalConfig), 0644)
	require.NoError(t, err)

	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)

	// Server defaults
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 30*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, 30*time.Second, cfg.Server.WriteTimeout)

	// Destination defaults
	dest := cfg.Destinations[0]
	assert.Equal(t, "POST", dest.Method)
	assert.Equal(t, "json", dest.Format)
	assert.Equal(t, "go-template", dest.Engine)
	assert.True(t, dest.Enabled)
	assert.Equal(t, 1, dest.ParallelRequests)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		expectError string
	}{
		{
			name: "invalid port",
			config: `
server:
  port: 99999
destinations:
  - name: "test"
    url: "https://example.com"
`,
			expectError: "invalid server port",
		},
		{
			name: "missing auth credentials",
			config: `
server:
  auth:
    enabled: true
destinations:
  - name: "test"
    url: "https://example.com"
`,
			expectError: "auth enabled but username or password not provided",
		},
		{
			name: "no destinations",
			config: `
server:
  port: 8080
`,
			expectError: "no destinations configured",
		},
		{
			name: "missing destination name",
			config: `
destinations:
  - url: "https://example.com"
`,
			expectError: "name is required",
		},
		{
			name: "duplicate destination name",
			config: `
destinations:
  - name: "test"
    url: "https://example.com"
    template: "test"
  - name: "test"
    url: "https://example.com"
    template: "test"
`,
			expectError: "duplicate destination name",
		},
		{
			name: "invalid destination name",
			config: `
destinations:
  - name: "test@invalid"
    url: "https://example.com"
`,
			expectError: "invalid name format",
		},
		{
			name: "missing url",
			config: `
destinations:
  - name: "test"
`,
			expectError: "url is required",
		},
		{
			name: "invalid method",
			config: `
destinations:
  - name: "test"
    url: "https://example.com"
    method: "INVALID"
`,
			expectError: "invalid method",
		},
		{
			name: "invalid format",
			config: `
destinations:
  - name: "test"
    url: "https://example.com"
    format: "xml"
`,
			expectError: "invalid format",
		},
		{
			name: "invalid engine",
			config: `
destinations:
  - name: "test"
    url: "https://example.com"
    engine: "invalid"
`,
			expectError: "invalid engine",
		},
		{
			name: "missing template for go-template",
			config: `
destinations:
  - name: "test"
    url: "https://example.com"
    engine: "go-template"
`,
			expectError: "template is required for go-template engine",
		},
		{
			name: "missing transform for jq",
			config: `
destinations:
  - name: "test"
    url: "https://example.com"
    engine: "jq"
`,
			expectError: "transform is required for jq engine",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")

			err := os.WriteFile(configPath, []byte(tt.config), 0644)
			require.NoError(t, err)

			_, err = LoadConfig(configPath)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectError)
		})
	}
}

// TestGetDestinationByPath removed - no longer using path field

func TestConfig_GetDestinationByName(t *testing.T) {
	cfg := &Config{
		Destinations: []DestinationConfig{
			{Name: "dest1", Enabled: true},
			{Name: "dest2", Enabled: true},
			{Name: "dest3", Enabled: false},
		},
	}

	// Test existing enabled destination
	dest := cfg.GetDestinationByName("dest1")
	require.NotNil(t, dest)
	assert.Equal(t, "dest1", dest.Name)

	// Test non-existing destination
	dest = cfg.GetDestinationByName("nonexistent")
	assert.Nil(t, dest)

	// Test disabled destination
	dest = cfg.GetDestinationByName("dest3")
	assert.Nil(t, dest)
}
