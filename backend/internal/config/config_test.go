package config

import (
	"strings"
	"testing"
)

func TestLoad_MissingRequiredVars(t *testing.T) {
	// Deliberately not setting any DB_* vars.
	_, err := Load()
	if err == nil {
		t.Fatal("expected an error when required DB_* variables are missing, got nil")
	}
	for _, want := range []string{"DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD", "JWT_SECRET"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected error to mention %s, got: %v", want, err)
		}
	}
}

func TestLoad_AllRequiredVarsSet(t *testing.T) {
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "lastmile")
	t.Setenv("DB_USER", "lastmile")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("JWT_SECRET", "test-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.DB.Host != "localhost" || cfg.DB.Port != "5432" {
		t.Errorf("unexpected DB config: %+v", cfg.DB)
	}
	if cfg.JWTSecret != "test-secret" {
		t.Errorf("JWTSecret = %q, want test-secret", cfg.JWTSecret)
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "lastmile")
	t.Setenv("DB_USER", "lastmile")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("JWT_SECRET", "test-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.AppEnv != "development" {
		t.Errorf("expected default AppEnv=development, got %q", cfg.AppEnv)
	}
	if cfg.ServerHost != "0.0.0.0" {
		t.Errorf("expected default ServerHost=0.0.0.0, got %q", cfg.ServerHost)
	}
	if cfg.ServerPort != "8080" {
		t.Errorf("expected default ServerPort=8080, got %q", cfg.ServerPort)
	}
	if cfg.DB.SSLMode != "disable" {
		t.Errorf("expected default DB.SSLMode=disable, got %q", cfg.DB.SSLMode)
	}
}

func TestLoad_NotificationsDefaults(t *testing.T) {
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "lastmile")
	t.Setenv("DB_USER", "lastmile")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("JWT_SECRET", "test-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Notifications.EmailProvider != "log" {
		t.Errorf("expected default EmailProvider=log, got %q", cfg.Notifications.EmailProvider)
	}
	if cfg.CORSAllowedOrigins != nil {
		t.Errorf("expected nil CORSAllowedOrigins by default, got %v", cfg.CORSAllowedOrigins)
	}
}

func TestLoad_ResendConfigured(t *testing.T) {
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "lastmile")
	t.Setenv("DB_USER", "lastmile")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("EMAIL_PROVIDER", "resend")
	t.Setenv("RESEND_API_KEY", "re_test_key")
	t.Setenv("RESEND_FROM_EMAIL", "orders@example.com")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com, https://admin.example.com ,")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Notifications.EmailProvider != "resend" || cfg.Notifications.ResendAPIKey != "re_test_key" || cfg.Notifications.ResendFromAddr != "orders@example.com" {
		t.Errorf("Notifications = %+v, want resend/re_test_key/orders@example.com", cfg.Notifications)
	}
	want := []string{"https://app.example.com", "https://admin.example.com"}
	if len(cfg.CORSAllowedOrigins) != len(want) || cfg.CORSAllowedOrigins[0] != want[0] || cfg.CORSAllowedOrigins[1] != want[1] {
		t.Errorf("CORSAllowedOrigins = %v, want %v (whitespace trimmed, blank entries dropped)", cfg.CORSAllowedOrigins, want)
	}
}

func TestConfig_DSN(t *testing.T) {
	cfg := &Config{
		DB: DatabaseConfig{
			Host:     "localhost",
			Port:     "5432",
			Name:     "lastmile",
			User:     "lastmile",
			Password: "p@ss/word",
			SSLMode:  "disable",
		},
	}
	dsn := cfg.DSN()
	want := "postgres://lastmile:p%40ss%2Fword@localhost:5432/lastmile?sslmode=disable"
	if dsn != want {
		t.Errorf("DSN() = %q, want %q", dsn, want)
	}
}
