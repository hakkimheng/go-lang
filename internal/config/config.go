// Package config loads runtime configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const devSecretFallback = "dev-secret-do-not-use-in-production-0123456789"

// Config holds every knob the server needs to boot.
type Config struct {
	Env             string
	Port            string
	JWTSecret       []byte
	JWTIssuer       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	ShutdownTimeout time.Duration
	// EnableDocs serves the OpenAPI document and Swagger UI.
	EnableDocs bool
}

// IsProduction reports whether the server should apply production-grade checks.
func (c Config) IsProduction() bool { return c.Env == "production" }

// Load reads configuration from the environment, applying development-friendly
// defaults. It returns an error when a value is missing or unusable so that the
// process fails at startup rather than at the first request.
func Load() (Config, error) {
	cfg := Config{
		Env:             envOr("APP_ENV", "development"),
		Port:            envOr("PORT", "8080"),
		JWTIssuer:       envOr("JWT_ISSUER", "restgo"),
		ShutdownTimeout: 10 * time.Second,
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		if cfg.IsProduction() {
			return Config{}, errors.New("JWT_SECRET is required when APP_ENV=production")
		}
		secret = devSecretFallback
	}
	if cfg.IsProduction() {
		if secret == devSecretFallback {
			return Config{}, errors.New("JWT_SECRET must not be the development fallback in production")
		}
		if len(secret) < 32 {
			return Config{}, errors.New("JWT_SECRET must be at least 32 bytes in production")
		}
	}
	cfg.JWTSecret = []byte(secret)

	var err error
	if cfg.AccessTokenTTL, err = durationOr("ACCESS_TOKEN_TTL", 15*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.RefreshTokenTTL, err = durationOr("REFRESH_TOKEN_TTL", 7*24*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.EnableDocs, err = boolOr("ENABLE_DOCS", true); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func boolOr(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s: must be true or false, got %q", key, raw)
	}
	return v, nil
}

func durationOr(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive, got %s", key, raw)
	}
	return d, nil
}
