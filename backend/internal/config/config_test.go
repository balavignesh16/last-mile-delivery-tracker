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
	for _, want := range []string{"DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD"} {
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

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.DB.Host != "localhost" || cfg.DB.Port != "5432" {
		t.Errorf("unexpected DB config: %+v", cfg.DB)
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "lastmile")
	t.Setenv("DB_USER", "lastmile")
	t.Setenv("DB_PASSWORD", "secret")

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
