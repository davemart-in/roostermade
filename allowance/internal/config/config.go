package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Port                   string
	DBPath                 string
	PrivacyAPIKey          string
	PrivacyBaseURL         string
	WebhookSecret          string
	NotificationWebhookURL string
	ApprovalTimeoutMinutes int
	LogLevel               string
}

func Load() (*Config, error) {
	_ = loadDotEnv(".env")

	home, _ := os.UserHomeDir()
	defaultDBPath := filepath.Join(home, ".allowance", "allowance.db")

	cfg := &Config{
		Port:                   getEnv("PORT", "3000"),
		DBPath:                 expandHome(getEnv("DB_PATH", defaultDBPath)),
		PrivacyAPIKey:          strings.TrimSpace(os.Getenv("PRIVACY_API_KEY")),
		PrivacyBaseURL:         getEnv("PRIVACY_BASE_URL", "https://api.privacy.com/v1"),
		WebhookSecret:          strings.TrimSpace(os.Getenv("WEBHOOK_SECRET")),
		NotificationWebhookURL: strings.TrimSpace(os.Getenv("NOTIFICATION_WEBHOOK_URL")),
		ApprovalTimeoutMinutes: 30,
		LogLevel:               getEnv("LOG_LEVEL", "info"),
	}

	if v := strings.TrimSpace(os.Getenv("APPROVAL_TIMEOUT_MINUTES")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid APPROVAL_TIMEOUT_MINUTES: %q", v)
		}
		cfg.ApprovalTimeoutMinutes = n
	}

	return cfg, nil
}

func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, strings.Trim(v, `"'`))
		}
	}
	return s.Err()
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/"))
}
