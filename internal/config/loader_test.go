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
		expectedAddress  string
		expectedPassword string
		expectedURL      string
		additionalChecks func(t *testing.T, cfg *Config)
	}{
		{
			name: "basic configuration",
			configContent: `
server:
  address: "127.0.0.1:8090"
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
			expectedAddress:  "127.0.0.1:8090",
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
  address: "localhost:8080"
  auth:
    enabled: true
    username: "admin"
    password: "default-pass"

destinations:
  - name: "env-test"
    url: "https://example.com/hook"
    format: "json"
    template: '{"test": "data"}'
`,
			envVars: map[string]string{
				"GATEWAY_SERVER_ADDRESS":       "192.168.1.1:9090",
				"GATEWAY_SERVER_AUTH_PASSWORD": "env-secret",
			},
			expectedAddress:  "192.168.1.1:9090",
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
			assert.Equal(t, tt.expectedAddress, cfg.Server.Address)
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

	// Server defaults - Address defaults to :8080 for dual stack (IPv4 and IPv6)
	assert.Equal(t, ":8080", cfg.Server.Address)
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

func TestLoadConfig_InvalidPath(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.yaml")
	assert.Error(t, err)
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	invalidYAML := `
server:
  address: invalid yaml content [[[
`

	err := os.WriteFile(configPath, []byte(invalidYAML), 0644)
	require.NoError(t, err)

	_, err = LoadConfig(configPath)
	assert.Error(t, err)
}
