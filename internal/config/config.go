package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config keeps runtime settings for the bot.
type Config struct {
	TelegramToken  string
	DatabaseURL    string
	ReportInterval time.Duration
}

// Load reads configuration from environment variables with sane defaults.
func Load() (Config, error) {
	cfg := Config{
		TelegramToken:  strings.TrimSpace(os.Getenv("TELEGRAM_TOKEN")),
		DatabaseURL:    strings.TrimSpace(os.Getenv("DATABASE_URL")),
		ReportInterval: parseInterval(strings.TrimSpace(os.Getenv("REPORT_INTERVAL_HOURS"))),
	}

	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = "daily_planner.db"
	}

	if cfg.ReportInterval == 0 {
		cfg.ReportInterval = 5 * time.Hour
	}

	if cfg.TelegramToken == "" {
		return cfg, fmt.Errorf("TELEGRAM_TOKEN is required")
	}

	return cfg, nil
}

func parseInterval(raw string) time.Duration {
	if raw == "" {
		return 0
	}
	hours, err := time.ParseDuration(raw + "h")
	if err != nil || hours <= 0 {
		return 0
	}
	return hours
}
