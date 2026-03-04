package config

import (
	"strings"
	"testing"
)

func TestLoad_AllVarsSet(t *testing.T) {
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_USER", "testuser")
	t.Setenv("DB_PASSWORD", "testpassword")
	t.Setenv("DB_NAME", "testdb")

	cfg, err := Load()

	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.DBHost != "localhost" {
		t.Errorf("expected DBHost to be 'localhost', got %s", cfg.DBHost)
	}

	if cfg.DBUser != "testuser" {
		t.Errorf("expected DBUser to be 'testuser', got %s", cfg.DBUser)
	}

	if cfg.DBPassword != "testpassword" {
		t.Errorf("expected DBPassword to be 'testpassword', got %s", cfg.DBPassword)
	}

	if cfg.DBName != "testdb" {
		t.Errorf("expected DBName to be 'testdb', got %s", cfg.DBName)
	}
}

func TestLoad_DefaultsApplied(t *testing.T) {
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_USER", "testuser")
	t.Setenv("DB_PASSWORD", "testpassword")
	t.Setenv("DB_NAME", "testdb")

	cfg, err := Load()

	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.HTTPPort != "8080" {
		t.Errorf("expected HTTPPort to be '8080', got %s", cfg.HTTPPort)
	}

	if cfg.DBPort != "5432" {
		t.Errorf("expected DBPort to be '5432', got %s", cfg.DBPort)
	}

	if cfg.DBSSLMode != "disable" {
		t.Errorf("expected DBSSLMode to be 'disable', got %s", cfg.DBSSLMode)
	}

	if cfg.LogLevel != "info" {
		t.Errorf("expected LogLevel to be 'info', got %s", cfg.LogLevel)
	}

	if cfg.LogFormat != "json" {
		t.Errorf("expected LogFormat to be 'json', got %s", cfg.LogFormat)
	}
}

func TestLoad_MissingOneRequired(t *testing.T) {
	t.Setenv("DB_HOST", "")
	t.Setenv("DB_USER", "testuser")
	t.Setenv("DB_PASSWORD", "testpassword")
	t.Setenv("DB_NAME", "testdb")

	_, err := Load()

	if err == nil {
		t.Fatalf("expected an error, got nil")
	}

	if !strings.Contains(err.Error(), "DB_HOST") {
		t.Errorf("expected error to contain 'DB_HOST', got %s", err.Error())
	}
}

func TestLoad_MissingMultipleRequired(t *testing.T) {
	t.Setenv("DB_HOST", "")
	t.Setenv("DB_USER", "")
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_NAME", "")

	_, err := Load()

	if err == nil {
		t.Fatalf("expected an error, got nil")
	}

	if !strings.Contains(err.Error(), "DB_HOST") {
		t.Errorf("expected error to contain 'DB_HOST', got %s", err.Error())
	}

	if !strings.Contains(err.Error(), "DB_USER") {
		t.Errorf("expected error to contain 'DB_USER', got %s", err.Error())
	}

	if !strings.Contains(err.Error(), "DB_PASSWORD") {
		t.Errorf("expected error to contain 'DB_PASSWORD', got %s", err.Error())
	}

	if !strings.Contains(err.Error(), "DB_NAME") {
		t.Errorf("expected error to contain 'DB_NAME', got %s", err.Error())
	}
}

func TestDatabaseUrl(t *testing.T) {
	cfg := Config{
		DBHost:     "localhost",
		DBPort:     "5432",
		DBUser:     "testuser",
		DBPassword: "testpassword",
		DBName:     "testdb",
		DBSSLMode:  "disable",
	}

	expectedURL := "host=localhost port=5432 user=testuser password=testpassword dbname=testdb sslmode=disable"

	if cfg.DatabaseURL() != expectedURL {
		t.Errorf("expected DatabaseURL to be %s, got %s", expectedURL, cfg.DatabaseURL())
	}
}

func TestGetSlogLogger(t *testing.T) {
	tests := []struct {
		name   string
		level  string
		format string
	}{
		{"error/text", "error", "text"},
		{"warn/json", "warn", "json"},
		{"info/text", "info", "text"},
		{"debug/json", "debug", "json"},
		{"default/default", "unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{LogLevel: tt.level, LogFormat: tt.format}
			if cfg.GetSlogLogger() == nil {
				t.Error("expected non-nil logger")
			}
		})
	}
}
