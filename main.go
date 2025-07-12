package main

import (
	"flag"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
	"github.com/vitalvas/alertmanager-gateway/internal/server"
)

func main() {
	var (
		configPath string
		logLevel   string
		logFormat  string
	)

	flag.StringVar(&configPath, "config", "config.yaml", "Path to configuration file")
	flag.StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	flag.StringVar(&logFormat, "log-format", "json", "Log format (json, text)")
	flag.Parse()

	// Setup logger
	logger := setupLogger(logLevel, logFormat)

	logger.Info("Alertmanager Gateway starting...")
	logger.WithField("path", configPath).Info("Loading configuration")

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		logger.WithError(err).Fatal("Failed to load configuration")
	}

	logger.WithFields(logrus.Fields{
		"server":       fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		"destinations": len(cfg.Destinations),
		"auth_enabled": cfg.Server.Auth.Enabled,
	}).Info("Configuration loaded successfully")

	// Log enabled destinations
	for _, dest := range cfg.Destinations {
		if dest.Enabled {
			logger.WithFields(logrus.Fields{
				"name":   dest.Name,
				"method": dest.Method,
				"path":   dest.Path,
				"engine": dest.Engine,
			}).Info("Destination configured")
		}
	}

	// Create and run server
	srv := server.New(cfg, logger)
	if err := srv.Run(); err != nil {
		logger.WithError(err).Fatal("Server failed")
	}
}

func setupLogger(level, format string) *logrus.Logger {
	logger := logrus.New()

	// Set log level
	switch level {
	case "debug":
		logger.SetLevel(logrus.DebugLevel)
	case "warn":
		logger.SetLevel(logrus.WarnLevel)
	case "error":
		logger.SetLevel(logrus.ErrorLevel)
	default:
		logger.SetLevel(logrus.InfoLevel)
	}

	// Set log format
	if format == "text" {
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	} else {
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z",
		})
	}

	return logger
}
