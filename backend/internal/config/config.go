// Package config loads application configuration from environment variables.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Config holds every setting the backend needs, across all modules.
type Config struct {
	AppEnv     string
	ServerHost string
	ServerPort string
	DB         DatabaseConfig
	JWTSecret  string
	// CORSAllowedOrigins is optional and empty by default — no CORS
	// header is ever added unless explicitly configured (see
	// internal/server.CORS). Needed only once the frontend and backend
	// are hosted on different origins; the dev setup avoids it entirely
	// via Vite's own proxy (see frontend/vite.config.ts).
	CORSAllowedOrigins []string
	Notifications      NotificationsConfig
}

// NotificationsConfig selects and configures M11's EmailProvider.
// Provider defaults to "log" (internal/notifications.LogEmailProvider,
// no credentials, no external call) — "resend" additionally requires
// ResendAPIKey/ResendFromEmail. SmsProvider stays log-only; nothing here
// changes internal/notifications' own dispatch/idempotency logic, only
// which EmailProvider implementation main.go constructs.
type NotificationsConfig struct {
	EmailProvider  string
	ResendAPIKey   string
	ResendFromAddr string
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	SSLMode  string
}

// Load reads configuration from environment variables. It returns a
// descriptive error naming every missing required variable rather than
// failing on the first one, so a misconfigured environment can be fixed
// in one pass.
func Load() (*Config, error) {
	cfg := &Config{
		AppEnv:     getenvDefault("APP_ENV", "development"),
		ServerHost: getenvDefault("SERVER_HOST", "0.0.0.0"),
		ServerPort: getenvDefault("SERVER_PORT", "8080"),
		DB: DatabaseConfig{
			Host:     os.Getenv("DB_HOST"),
			Port:     os.Getenv("DB_PORT"),
			Name:     os.Getenv("DB_NAME"),
			User:     os.Getenv("DB_USER"),
			Password: os.Getenv("DB_PASSWORD"),
			SSLMode:  getenvDefault("DB_SSLMODE", "disable"),
		},
		JWTSecret:          os.Getenv("JWT_SECRET"),
		CORSAllowedOrigins: parseOriginList(os.Getenv("CORS_ALLOWED_ORIGINS")),
		Notifications: NotificationsConfig{
			EmailProvider:  getenvDefault("EMAIL_PROVIDER", "log"),
			ResendAPIKey:   os.Getenv("RESEND_API_KEY"),
			ResendFromAddr: os.Getenv("RESEND_FROM_EMAIL"),
		},
	}

	var missing []string
	if cfg.DB.Host == "" {
		missing = append(missing, "DB_HOST")
	}
	if cfg.DB.Port == "" {
		missing = append(missing, "DB_PORT")
	}
	if cfg.DB.Name == "" {
		missing = append(missing, "DB_NAME")
	}
	if cfg.DB.User == "" {
		missing = append(missing, "DB_USER")
	}
	if cfg.DB.Password == "" {
		missing = append(missing, "DB_PASSWORD")
	}
	if cfg.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variable(s): %s (see .env.example)", strings.Join(missing, ", "))
	}

	return cfg, nil
}

// DSN builds a PostgreSQL connection string from the database configuration.
func (c *Config) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		url.QueryEscape(c.DB.User),
		url.QueryEscape(c.DB.Password),
		c.DB.Host,
		c.DB.Port,
		c.DB.Name,
		c.DB.SSLMode,
	)
}

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseOriginList splits a comma-separated CORS_ALLOWED_ORIGINS value
// into a clean slice — empty/unset input yields nil (no CORS headers at
// all, see internal/server.CORS), and any blank entry between commas is
// dropped rather than becoming a spurious empty-string "allowed origin".
func parseOriginList(raw string) []string {
	if raw == "" {
		return nil
	}
	var origins []string
	for _, o := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(o); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	return origins
}
