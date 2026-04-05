package config

import (
	"strings"
	"testing"
	"time"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_USER", "testuser")
	t.Setenv("DB_PASSWORD", "testpassword")
	t.Setenv("DB_NAME", "testdb")
}

func clearOptionalEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HTTP_PORT", "")
	t.Setenv("DB_PORT", "")
	t.Setenv("DB_SSL_MODE", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LOG_FORMAT", "")
	t.Setenv("WORKER_POOL_SIZE", "")
	t.Setenv("WORKER_POLL_INTERVAL", "")
	t.Setenv("WORKER_BATCH_SIZE", "")
	t.Setenv("WORKER_TASK_TIMEOUT", "")
	t.Setenv("WORKER_STALE_INTERVAL", "")
	t.Setenv("WORKER_STALE_DURATION", "")
	t.Setenv("SHUTDOWN_TIMEOUT", "")
	t.Setenv("INSTANCE_MONITOR_INTERVAL", "")
	t.Setenv("HEARTBEAT_TIMEOUT", "")
	t.Setenv("HEARTBEAT_INTERVAL", "")
	t.Setenv("LEADER_INTERVAL", "")
	t.Setenv("KAFKA_BROKERS", "")
	t.Setenv("KAFKA_TOPIC", "")
	t.Setenv("KAFKA_GROUP_ID", "")
	t.Setenv("OUTBOX_POLL_INTERVAL", "")
	t.Setenv("OUTBOX_BATCH_SIZE", "")
	t.Setenv("OUTBOX_RETENTION", "")
	t.Setenv("OTEL_COLLECTOR_ENDPOINT", "")
	t.Setenv("OTEL_SERVICE_NAME", "")
	t.Setenv("OTEL_EXPORT_INTERVAL", "")
}

func TestLoad_AllVarsSet(t *testing.T) {
	setRequiredEnv(t)

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
	setRequiredEnv(t)
	clearOptionalEnv(t)

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

	if cfg.WorkerPoolSize != 5 {
		t.Errorf("expected WorkerPoolSize to be 5, got %d", cfg.WorkerPoolSize)
	}

	if cfg.WorkerPollInterval != 30*time.Second {
		t.Errorf("expected WorkerPollInterval to be 30 seconds, got %v", cfg.WorkerPollInterval)
	}

	if cfg.WorkerBatchSize != 10 {
		t.Errorf("expected WorkerBatchSize to be 10, got %d", cfg.WorkerBatchSize)
	}

	if cfg.WorkerTaskTimeout != 30*time.Second {
		t.Errorf("expected WorkerTaskTimeout to be 30 seconds, got %v", cfg.WorkerTaskTimeout)
	}

	if cfg.WorkerStaleInterval != 30*time.Second {
		t.Errorf("expected WorkerStaleInterval to be 30 seconds, got %v", cfg.WorkerStaleInterval)
	}

	if cfg.WorkerStaleDuration != 5*time.Minute {
		t.Errorf("expected WorkerStaleDuration to be 5 minutes, got %v", cfg.WorkerStaleDuration)
	}

	if cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("expected ShutdownTimeout to be 30 seconds, got %v", cfg.ShutdownTimeout)
	}

	if cfg.InstanceMonitorInterval != 30*time.Second {
		t.Errorf("expected InstanceMonitorInterval to be 30 seconds, got %v", cfg.InstanceMonitorInterval)
	}

	if cfg.HeartbeatTimeout != 15*time.Second {
		t.Errorf("expected HeartbeatTimeout to be 15 seconds, got %v", cfg.HeartbeatTimeout)
	}

	if cfg.HeartbeatInterval != 5*time.Second {
		t.Errorf("expected HeartbeatInterval to be 5 seconds, got %v", cfg.HeartbeatInterval)
	}

	if cfg.LeaderInterval != 10*time.Second {
		t.Errorf("expected LeaderInterval to be 10 seconds, got %v", cfg.LeaderInterval)
	}

	if len(cfg.KafkaBrokers) != 1 || cfg.KafkaBrokers[0] != "localhost:9092" {
		t.Errorf("expected KafkaBrokers to be [localhost:9092], got %v", cfg.KafkaBrokers)
	}

	if cfg.KafkaTopic != "tasks" {
		t.Errorf("expected KafkaTopic to be 'tasks', got %s", cfg.KafkaTopic)
	}

	if cfg.KafkaGroupID != "taskforge-workers" {
		t.Errorf("expected KafkaGroupID to be 'taskforge-workers', got %s", cfg.KafkaGroupID)
	}

	if cfg.OutboxPollInterval != 1*time.Second {
		t.Errorf("expected OutboxPollInterval to be 1 second, got %v", cfg.OutboxPollInterval)
	}

	if cfg.OutboxBatchSize != 50 {
		t.Errorf("expected OutboxBatchSize to be 50, got %d", cfg.OutboxBatchSize)
	}

	if cfg.OutboxRetention != 24*time.Hour {
		t.Errorf("expected OutboxRetention to be 24 hours, got %v", cfg.OutboxRetention)
	}

	if cfg.OtelCollectorEndpoint != "localhost:4317" {
		t.Errorf("expected OtelCollectorEndpoint to be 'localhost:4317', got %s", cfg.OtelCollectorEndpoint)
	}

	if cfg.OtelServiceName != "taskforge" {
		t.Errorf("expected OtelServiceName to be 'taskforge', got %s", cfg.OtelServiceName)
	}

	if cfg.OtelExportInterval != 15*time.Second {
		t.Errorf("expected OtelExportInterval to be 15 seconds, got %v", cfg.OtelExportInterval)
	}
}

func TestLoad_DefaultsOverride(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HTTP_PORT", "433")
	t.Setenv("WORKER_POOL_SIZE", "50")
	t.Setenv("WORKER_POLL_INTERVAL", "10s")

	cfg, err := Load()

	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.HTTPPort != "433" {
		t.Errorf("expected HTTPPort to be '433', got %s", cfg.HTTPPort)
	}

	if cfg.WorkerPoolSize != 50 {
		t.Errorf("expected WorkerPoolSize to be 50, got %d", cfg.WorkerPoolSize)
	}

	if cfg.WorkerPollInterval != 10*time.Second {
		t.Errorf("expected WorkerPollInterval to be 10 seconds, got %v", cfg.WorkerPollInterval)
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

func TestLoad_KafkaBrokersMultiple(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("KAFKA_BROKERS", "broker1:9092, broker2:9092, broker3:9092")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.KafkaBrokers) != 3 {
		t.Fatalf("expected 3 brokers, got %d", len(cfg.KafkaBrokers))
	}

	if cfg.KafkaBrokers[0] != "broker1:9092" {
		t.Errorf("expected first broker 'broker1:9092', got '%s'", cfg.KafkaBrokers[0])
	}

	if cfg.KafkaBrokers[1] != "broker2:9092" {
		t.Errorf("expected second broker 'broker2:9092' (trimmed), got '%s'", cfg.KafkaBrokers[1])
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
