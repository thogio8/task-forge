package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	HTTPPort   string
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string
}

func Load() (Config, error) {
	cfg := Config{}
	var missing []string

	// Optionnal fields
	cfg.HTTPPort = getEnvOrDefault("HTTP_PORT", "8080")
	cfg.DBPort = getEnvOrDefault("DB_PORT", "5432")
	cfg.DBSSLMode = getEnvOrDefault("DB_SSL_MODE", "disable")

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

func getEnvOrDefault(key, defaultValue string) string {
	envValue := os.Getenv(key)

	if envValue != "" {
		return envValue
	}

	return defaultValue
}

func getEnvRequired(key string, missing []string) (string, []string) {
	envValue := os.Getenv(key)

	if envValue == "" {
		missing = append(missing, key)
	}

	return envValue, missing
}
