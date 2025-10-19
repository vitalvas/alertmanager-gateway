package config

import (
	"fmt"
	"net/http"
)

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate server config
	if c.Server.Address == "" {
		return fmt.Errorf("server address is required")
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

	for i, dest := range c.Destinations {
		if dest.Name == "" {
			return fmt.Errorf("destination %d: name is required", i)
		}

		// Validate name format (alphanumeric, dash, underscore)
		if !isValidDestinationName(dest.Name) {
			return fmt.Errorf("destination %s: invalid name format (use alphanumeric, dash, or underscore)", dest.Name)
		}

		if destNames[dest.Name] {
			return fmt.Errorf("duplicate destination name: %s", dest.Name)
		}
		destNames[dest.Name] = true

		if dest.URL == "" {
			return fmt.Errorf("destination %s: url is required", dest.Name)
		}

		validMethods := map[string]bool{
			http.MethodGet:    true,
			http.MethodPost:   true,
			http.MethodPut:    true,
			http.MethodPatch:  true,
			http.MethodDelete: true,
		}
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
	}

	return nil
}

// isValidDestinationName checks if a destination name is valid
func isValidDestinationName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}
