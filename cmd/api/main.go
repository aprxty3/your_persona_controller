package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/config"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	echo "github.com/labstack/echo/v4"
)

// Typed aliases to avoid Wire's type ambiguity with multiple string arguments.
type GeminiAPIKey string
type GeminiModel string
type DBDSN string
type RedisAddr string
type RedisPassword string
type JWTSecret string
type S3Endpoint string
type S3Region string
type S3Bucket string
type S3AccessKey string
type S3SecretKey string
type S3UsePathStyle bool
type TurnstileSecretKey string
type IsProduction bool
type AllowedOrigins string
type TrustedProxies string

// @title Your Persona API
// @version 1.0
// @description API Server for Your Persona psychological assessment platform.
// @host localhost:8080
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Masukkan access token tanpa prefix "Bearer ", cukup token JWT-nya saja.
func main() {
	// ---------------------------------------------------------
	// ENVIRONMENT VARIABLES
	// ---------------------------------------------------------
	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	geminiModel := config.EnvOr("GEMINI_MODEL", "gemini-2.5-pro-001")

	maxConcurrent, err := strconv.ParseInt(os.Getenv("GEMINI_MAX_CONCURRENT"), 10, 64)
	if err != nil || maxConcurrent <= 0 {
		maxConcurrent = 10
	}

	dbDSN := config.PostgresDSN()

	redisAddr := config.RedisAddr()
	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisDB, _ := strconv.Atoi(os.Getenv("REDIS_DB")) // default 0

	jwtSecret := config.EnvOr("JWT_SECRET", "your-persona-super-secret-key-change-in-production-123456")

	// Object storage (MinIO dev / R2 prod) — result PDF signed-URL downloads.
	s3Endpoint := config.EnvOr("S3_ENDPOINT", "http://localhost:9000")
	s3Region := config.EnvOr("S3_REGION", "auto")
	s3Bucket := config.EnvOr("S3_BUCKET", "your-personas-reports")
	s3AccessKey := config.EnvOr("S3_ACCESS_KEY", "minioadmin")
	s3SecretKey := config.EnvOr("S3_SECRET_KEY", "minioadmin")
	s3UsePathStyle, _ := strconv.ParseBool(config.EnvOr("S3_USE_PATH_STYLE", "true"))

	appEnv := config.EnvOr("APP_ENV", "development")
	logInstance := logger.NewLogger(appEnv)
	isProduction := appEnv == "production"
	turnstileSecretKey := os.Getenv("TURNSTILE_SECRET_KEY")
	allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	trustedProxies := os.Getenv("TRUSTED_PROXIES")

	// refuse to boot in production with insecure/missing config
	if isProduction {
		config.RequireProduction(logInstance,
			config.Check{Name: "JWT_SECRET", Value: jwtSecret, InsecureDefault: "your-persona-super-secret-key-change-in-production-123456"},
			config.Check{Name: "GEMINI_API_KEY", Value: geminiAPIKey},
			config.Check{Name: "GEMINI_MODEL", Value: geminiModel},
			config.Check{Name: "DB_PASSWORD", Value: os.Getenv("DB_PASSWORD"), InsecureDefault: "changeme"},
			config.Check{Name: "S3_ACCESS_KEY", Value: s3AccessKey, InsecureDefault: "minioadmin"},
			config.Check{Name: "S3_SECRET_KEY", Value: s3SecretKey, InsecureDefault: "minioadmin"},
			config.Check{Name: "TURNSTILE_SECRET_KEY", Value: turnstileSecretKey},
			config.Check{Name: "ALLOWED_ORIGINS", Value: allowedOrigins},
		)
	}

	// ---------------------------------------------------------
	// DEPENDENCY INJECTION (Wire)
	// ---------------------------------------------------------
	var app *echo.Echo
	app, err = InitializeAPI(
		GeminiAPIKey(geminiAPIKey),
		GeminiModel(geminiModel),
		maxConcurrent,
		DBDSN(dbDSN),
		RedisAddr(redisAddr),
		RedisPassword(redisPassword),
		redisDB,
		JWTSecret(jwtSecret),
		S3Endpoint(s3Endpoint),
		S3Region(s3Region),
		S3Bucket(s3Bucket),
		S3AccessKey(s3AccessKey),
		S3SecretKey(s3SecretKey),
		S3UsePathStyle(s3UsePathStyle),
		TurnstileSecretKey(turnstileSecretKey),
		IsProduction(isProduction),
		AllowedOrigins(allowedOrigins),
		TrustedProxies(trustedProxies),
		logInstance,
	)
	if err != nil {
		logInstance.Error("failed to initialize API", "error", err)
		os.Exit(1)
	}

	port := config.EnvOr("APP_PORT", "8080")

	// ---------------------------------------------------------
	// SERVER START & GRACEFUL SHUTDOWN
	// ---------------------------------------------------------
	go func() {
		logInstance.Info("server is starting", "port", port)
		if err := app.Start(":" + port); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logInstance.Error("server forced to shutdown abruptly", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)

	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Block the main thread until a signal is received
	<-quit
	logInstance.Info("interrupt signal received, shutting down server gracefully")

	// Parse shutdown timeout from env, fallback to 30s
	timeoutStr := os.Getenv("SHUTDOWN_TIMEOUT")
	timeoutDuration, err := time.ParseDuration(timeoutStr)
	if err != nil || timeoutDuration == 0 {
		timeoutDuration = 30 * time.Second
	}

	// Create a context with the timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDuration)
	defer cancel()

	if err := app.Shutdown(ctx); err != nil {
		logInstance.Error("server shutdown failed or timed out", "error", err)
		os.Exit(1)
	}

	logInstance.Info("server exited properly")
}
