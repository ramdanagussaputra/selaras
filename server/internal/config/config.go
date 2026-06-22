// Package config loads 12-factor configuration from environment variables
// only, with fail-fast aggregate validation at boot (design D6).
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Environment names accepted in ENV.
const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
)

// Auth configuration defaults and bounds (M2 spec 02-auth §Dependencies).
const (
	minJWTSecretBytes      = 32
	defaultAccessTokenTTL  = 15 * time.Minute
	defaultRefreshTokenTTL = 168 * time.Hour
)

// Config is the process configuration. All values come from env vars.
type Config struct {
	Port            int           // PORT, default 8080
	DatabaseURL     string        // DATABASE_URL, required
	Env             string        // ENV, "development" (default) or "production"
	CORSOrigin      string        // CORS_ORIGIN, optional; empty disables CORS headers
	JWTSecret       string        // JWT_SECRET, required, >= 32 bytes
	AccessTokenTTL  time.Duration // ACCESS_TOKEN_TTL, default 15m
	RefreshTokenTTL time.Duration // REFRESH_TOKEN_TTL, default 168h
}

// Load reads and validates every variable, collecting all problems into one
// error so a misconfigured boot reports the full list at once.
func Load() (Config, error) {
	var errs []error

	configuration := Config{
		Port:            8080,
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		Env:             EnvDevelopment,
		CORSOrigin:      os.Getenv("CORS_ORIGIN"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		AccessTokenTTL:  defaultAccessTokenTTL,
		RefreshTokenTTL: defaultRefreshTokenTTL,
	}

	if raw := os.Getenv("PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		isValidPort := err == nil && port > 0 && port <= 65535
		if isValidPort {
			configuration.Port = port
		} else {
			errs = append(errs, fmt.Errorf("PORT must be an integer in 1-65535, got %q", raw))
		}
	}

	if configuration.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}

	if raw := os.Getenv("ENV"); raw != "" {
		switch raw {
		case EnvDevelopment, EnvProduction:
			configuration.Env = raw
		default:
			errs = append(errs, fmt.Errorf("ENV must be %q or %q, got %q", EnvDevelopment, EnvProduction, raw))
		}
	}

	if len(configuration.JWTSecret) < minJWTSecretBytes {
		errs = append(errs, fmt.Errorf("JWT_SECRET is required and must be at least %d bytes", minJWTSecretBytes))
	}

	if raw := os.Getenv("ACCESS_TOKEN_TTL"); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err == nil && ttl > 0 {
			configuration.AccessTokenTTL = ttl
		} else {
			errs = append(errs, fmt.Errorf("ACCESS_TOKEN_TTL must be a positive duration (e.g. 15m), got %q", raw))
		}
	}

	if raw := os.Getenv("REFRESH_TOKEN_TTL"); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err == nil && ttl > 0 {
			configuration.RefreshTokenTTL = ttl
		} else {
			errs = append(errs, fmt.Errorf("REFRESH_TOKEN_TTL must be a positive duration (e.g. 168h), got %q", raw))
		}
	}

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}

	return configuration, nil
}
