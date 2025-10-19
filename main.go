package main

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
	"github.com/vitalvas/alertmanager-gateway/internal/server"
)

var (
	configPath string
	logLevel   string
	logFormat  string
)

var rootCmd = &cobra.Command{
	Use:   "alertmanager-gateway",
	Short: "Universal adapter for Prometheus Alertmanager webhooks",
	Long:  "Universal adapter for Prometheus Alertmanager webhooks that transforms and routes alerts to various third-party notification systems.",
	Run:   run,
}

func init() {
	rootCmd.Flags().StringVarP(&configPath, "config", "c", "config.yaml", "Path to configuration file")
	rootCmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.Flags().StringVar(&logFormat, "log-format", "json", "Log format (json, text)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(_ *cobra.Command, _ []string) {
	logger := setupLogger(logLevel, logFormat)

	logger.Info("Alertmanager Gateway starting...")
	logger.WithField("path", configPath).Info("Loading configuration")

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		logger.WithError(err).Fatal("Failed to load configuration")
	}

	logger.WithFields(logrus.Fields{
		"address":      cfg.Server.Address,
		"destinations": len(cfg.Destinations),
		"auth_enabled": cfg.Server.Auth.Enabled,
	}).Info("Configuration loaded successfully")

	for _, dest := range cfg.Destinations {
		if dest.Enabled {
			logger.WithFields(logrus.Fields{
				"name":   dest.Name,
				"method": dest.Method,
				"url":    "/webhook/" + dest.Name,
				"engine": dest.Engine,
			}).Info("Destination configured")
		}
	}

	srv, err := server.New(cfg, logger)
	if err != nil {
		logger.WithError(err).Fatal("Failed to create server")
	}

	if err := srv.Run(); err != nil {
		logger.WithError(err).Fatal("Server failed")
	}
}

func setupLogger(level, format string) *logrus.Logger {
	logger := logrus.New()

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
