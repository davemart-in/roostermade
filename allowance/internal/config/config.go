package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	PrivacyAPIKey           string
	Port                    string
	DBPath                  string
	WebhookSecret           string
	NotificationWebhookURL  string
	ApprovalTimeoutMinutes  int
	LogLevel                string
}

func Load() Config {
	_ = godotenv.Load()

	privacyAPIKey := strings.TrimSpace(os.Getenv("PRIVACY_API_KEY"))
	if privacyAPIKey == "" {
		panic("missing required environment variable PRIVACY_API_KEY. Set it in .env or your shell.")
	}

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "3000"
	}

	dbPath := strings.TrimSpace(os.Getenv("DB_PATH"))
	if dbPath == "" {
		dbPath = "~/.allowance/allowance.db"
	}
	dbPath = expandTilde(dbPath)

	webhookSecret := strings.TrimSpace(os.Getenv("WEBHOOK_SECRET"))
	notificationWebhookURL := strings.TrimSpace(os.Getenv("NOTIFICATION_WEBHOOK_URL"))

	approvalTimeoutMinutes := 30
	if raw := strings.TrimSpace(os.Getenv("APPROVAL_TIMEOUT_MINUTES")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			panic(fmt.Sprintf("invalid APPROVAL_TIMEOUT_MINUTES: %q", raw))
		}
		approvalTimeoutMinutes = value
	}

	logLevel := strings.TrimSpace(os.Getenv("LOG_LEVEL"))
	if logLevel == "" {
		logLevel = "info"
	}

	return Config{
		PrivacyAPIKey:          privacyAPIKey,
		Port:                   port,
		DBPath:                 dbPath,
		WebhookSecret:          webhookSecret,
		NotificationWebhookURL: notificationWebhookURL,
		ApprovalTimeoutMinutes: approvalTimeoutMinutes,
		LogLevel:               logLevel,
	}
}

func expandTilde(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
