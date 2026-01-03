package config

import (
	"flag"
	"os"

	"github.com/joho/godotenv"
)

// Config holds the application configuration
type Config struct {
	Addr     string
	DBPath   string
	LogLevel string
}

// Load loads the configuration from environment variables and flags
func Load() (*Config, error) {
	// Load .env file if it exists
	_ = godotenv.Load()

	cfg := &Config{}

	// Define flags
	flag.StringVar(&cfg.Addr, "addr", getEnv("SERVIO_ADDR", ":8080"), "HTTP server address")
	flag.StringVar(&cfg.DBPath, "db", getEnv("SERVIO_DB", "servio.db"), "SQLite database path")
	flag.StringVar(&cfg.LogLevel, "log-level", getEnv("SERVIO_LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")

	flag.Parse()

	return cfg, nil
}

// getEnv returns the value of an environment variable or a default value
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
