// Package config provides typed environment-variable helpers and the
// RequireProduction boot-time safety check.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// EnvOr returns the value of the environment variable key, or fallback when
// it is unset/empty.
func EnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// EnvInt returns the environment variable key parsed as int, or fallback when
// it is unset/empty/not a positive integer.
func EnvInt(key string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

// PostgresDSN resolves the database DSN every binary (api, worker, migrate,
// seed) connects with: DB_DSN verbatim when set, otherwise assembled from the
// individual DB_* variables with the dev-compose defaults.
func PostgresDSN() string {
	if dsn := os.Getenv("DB_DSN"); dsn != "" {
		return dsn
	}
	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=Asia/Jakarta",
		EnvOr("DB_HOST", "localhost"),
		EnvOr("DB_USER", "postgres"),
		EnvOr("DB_PASSWORD", "changeme"),
		EnvOr("DB_NAME", "psyche_assessment"),
		EnvOr("DB_PORT", "5432"),
		EnvOr("DB_SSLMODE", "disable"),
	)
}

// RedisAddr resolves the Redis address: REDIS_ADDR verbatim when set,
// otherwise host:port from REDIS_HOST/REDIS_PORT with dev defaults.
func RedisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return EnvOr("REDIS_HOST", "localhost") + ":" + EnvOr("REDIS_PORT", "6379")
}
