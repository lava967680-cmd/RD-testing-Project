package config

import (
	"fmt"
	"log"
	"os"
)

type Config struct {
	Port           string
	DatabaseURL    string
	RedisURL       string
	JWTSecret      string
	Environment    string
	AllowedOrigins string
}

// Load reads configuration from environment variables.
// In production, missing security-critical values cause the process to exit
// rather than silently falling back to well-known development defaults.
func Load() *Config {
	env := getEnv("ENVIRONMENT", "development")
	isProd := env == "production"

	cfg := &Config{
		Port:        getEnv("PORT", "8080"),
		Environment: env,
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379/0"),
	}

	cfg.DatabaseURL = requireOrDefault("DATABASE_URL",
		"postgres://controlhub:controlhub_dev_secret_password@localhost:5432/controlhub_db?sslmode=disable",
		isProd)

	cfg.JWTSecret = requireOrDefault("JWT_SECRET",
		"dev_secret_jwt_key_at_least_32_bytes_long_entropy",
		isProd)

	cfg.AllowedOrigins = requireOrDefault("CORS_ALLOWED_ORIGINS",
		"http://localhost:3000",
		isProd)

	if isProd && len(cfg.JWTSecret) < 32 {
		log.Fatal("FATAL: JWT_SECRET must be at least 32 characters in production")
	}

	return cfg
}

// requireOrDefault returns the env var if set. In production, a missing
// value is a fatal error instead of silently using devDefault, since
// devDefault values are public (checked into git) and unsafe to run with.
func requireOrDefault(key, devDefault string, isProd bool) string {
	val := os.Getenv(key)
	if val != "" {
		return val
	}
	if isProd {
		log.Fatal(fmt.Sprintf("FATAL: %s must be set via environment variable in production. Refusing to start with an insecure default.", key))
	}
	return devDefault
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
