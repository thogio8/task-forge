package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPPort            string
	DBHost              string
	DBPort              string
	DBUser              string
	DBPassword          string
	DBName              string
	DBSSLMode           string
	LogLevel            string
	LogFormat           string
	WorkerPoolSize      int
	WorkerPollInterval  time.Duration
	WorkerBatchSize     int
	WorkerTaskTimeout   time.Duration
	WorkerStaleInterval time.Duration
	WorkerStaleDuration time.Duration
}

func Load() (Config, error) {
	cfg := Config{}
	var missing []string

	// Optional fields
	cfg.HTTPPort = getEnvOrDefault("HTTP_PORT", "8080")
	cfg.DBPort = getEnvOrDefault("DB_PORT", "5432")
	cfg.DBSSLMode = getEnvOrDefault("DB_SSL_MODE", "disable")
	cfg.LogLevel = getEnvOrDefault("LOG_LEVEL", "info")
	cfg.LogFormat = getEnvOrDefault("LOG_FORMAT", "json")
	cfg.WorkerPoolSize = getEnvOrDefaultInt("WORKER_POOL_SIZE", 5)
	cfg.WorkerPollInterval = getEnvOrDefaultDuration("WORKER_POLL_INTERVAL", 2*time.Second)
	cfg.WorkerBatchSize = getEnvOrDefaultInt("WORKER_BATCH_SIZE", 10)
	cfg.WorkerTaskTimeout = getEnvOrDefaultDuration("WORKER_TASK_TIMEOUT", 30*time.Second)
	cfg.WorkerStaleInterval = getEnvOrDefaultDuration("WORKER_STALE_INTERVAL", 30*time.Second)
	cfg.WorkerStaleDuration = getEnvOrDefaultDuration("WORKER_STALE_DURATION", 5*time.Minute)

	// Required fields
	cfg.DBHost, missing = getEnvRequired("DB_HOST", missing)
	cfg.DBUser, missing = getEnvRequired("DB_USER", missing)
	cfg.DBPassword, missing = getEnvRequired("DB_PASSWORD", missing)
	cfg.DBName, missing = getEnvRequired("DB_NAME", missing)

	if len(missing) > 0 {
		errorMessage := strings.Join(missing, ", ")
		err := fmt.Errorf("missing required environment variables : %s", errorMessage)
		return Config{}, err
	}

	return cfg, nil
}

func (c *Config) DatabaseURL() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost,
		c.DBPort,
		c.DBUser,
		c.DBPassword,
		c.DBName,
		c.DBSSLMode,
	)
}

func (c *Config) GetSlogLogger() *slog.Logger {
	var logLevel slog.Level
	var logHandler slog.Handler

	switch c.LogLevel {
	case "error":
		logLevel = slog.LevelError
	case "warn":
		logLevel = slog.LevelWarn
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	default:
		logLevel = slog.LevelInfo
	}

	logHandlerOpts := slog.HandlerOptions{Level: logLevel}

	switch c.LogFormat {
	case "text":
		logHandler = slog.NewTextHandler(os.Stdout, &logHandlerOpts)
	case "json":
		logHandler = slog.NewJSONHandler(os.Stdout, &logHandlerOpts)
	default:
		logHandler = slog.NewJSONHandler(os.Stdout, &logHandlerOpts)
	}

	return slog.New(logHandler)
}

func getEnvOrDefault(key, defaultValue string) string {
	envValue := os.Getenv(key)

	if envValue != "" {
		return envValue
	}

	return defaultValue
}

func getEnvOrDefaultInt(key string, defaultValue int) int {
	envValue := os.Getenv(key)

	if envValue == "" {
		return defaultValue
	}

	result, err := strconv.Atoi(envValue)

	if err != nil {
		return defaultValue
	}

	return result
}

func getEnvOrDefaultDuration(key string, defaultValue time.Duration) time.Duration {
	envValue := os.Getenv(key)

	if envValue == "" {
		return defaultValue
	}

	result, err := time.ParseDuration(envValue)

	if err != nil {
		return defaultValue
	}

	return result
}

func getEnvRequired(key string, missing []string) (string, []string) {
	envValue := os.Getenv(key)

	if envValue == "" {
		missing = append(missing, key)
	}

	return envValue, missing
}
