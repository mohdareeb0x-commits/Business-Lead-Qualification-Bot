package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	ServerPort    string
	ServerEnv     string
	MongoURI      string
	MongoDB       string
	TelegramToken string
	AdminChatID   int64
	WebhookSecret string
	GinMode       string
}

// Load reads environment variables (with .env support) and returns a Config
// or an error if any required value is missing or invalid.
func Load() (*Config, error) {
	// .env is optional in production. Ignore "not found" errors.
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			return nil, fmt.Errorf("load .env: %w", err)
		}
	}

	cfg := &Config{
		ServerPort:    getEnv("SERVER_PORT", "8080"),
		ServerEnv:     getEnv("SERVER_ENV", "development"),
		MongoURI:      os.Getenv("MONGODB_URI"),
		MongoDB:       getEnv("MONGODB_DATABASE", "telegram_leads"),
		TelegramToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		WebhookSecret: os.Getenv("TELEGRAM_WEBHOOK_SECRET"),
		GinMode:       getEnv("GIN_MODE", "release"),
	}

	if strings.TrimSpace(cfg.MongoURI) == "" {
		return nil, errors.New("MONGODB_URI is required")
	}

	adminRaw := os.Getenv("TELEGRAM_ADMIN_CHAT_ID")
	if strings.TrimSpace(adminRaw) == "" {
		return nil, errors.New("TELEGRAM_ADMIN_CHAT_ID is required")
	}
	adminID, err := strconv.ParseInt(adminRaw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("TELEGRAM_ADMIN_CHAT_ID invalid: %w", err)
	}
	cfg.AdminChatID = adminID

	if strings.TrimSpace(cfg.TelegramToken) == "" {
		// Token is required to send admin notifications and process updates.
		// Webhook will reject requests without TELEGRAM_WEBHOOK_SECRET anyway.
		slog.Warn("TELEGRAM_BOT_TOKEN is empty; bot features will be limited")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}
