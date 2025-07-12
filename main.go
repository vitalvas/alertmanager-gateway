package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/vitalvas/alertmanager-gateway/internal/config"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.yaml", "Path to configuration file")
	flag.Parse()

	fmt.Printf("Alertmanager Gateway starting...\n")
	fmt.Printf("Loading configuration from: %s\n", configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Configuration loaded successfully\n")
	fmt.Printf("Server will listen on %s:%d\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("Loaded %d destination(s)\n", len(cfg.Destinations))

	for _, dest := range cfg.Destinations {
		if dest.Enabled {
			fmt.Printf("  - %s: %s %s\n", dest.Name, dest.Method, dest.Path)
		}
	}

	// TODO: Start HTTP server
	os.Exit(0)
}
