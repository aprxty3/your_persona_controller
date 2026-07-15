package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

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
	geminiModel := os.Getenv("GEMINI_MODEL")
	if geminiModel == "" {
		geminiModel = "gemini-2.5-pro-001"
	}

	maxConcurrentStr := os.Getenv("GEMINI_MAX_CONCURRENT")
	maxConcurrent, err := strconv.ParseInt(maxConcurrentStr, 10, 64)
	if err != nil || maxConcurrent <= 0 {
		maxConcurrent = 10
	}

	dbDSN := os.Getenv("DB_DSN")
	if dbDSN == "" {
		dbHost := os.Getenv("DB_HOST")
		if dbHost == "" {
			dbHost = "localhost"
		}
		dbPort := os.Getenv("DB_PORT")
		if dbPort == "" {
			dbPort = "5432"
		}
		dbUser := os.Getenv("DB_USER")
		if dbUser == "" {
			dbUser = "postgres"
		}
		dbPassword := os.Getenv("DB_PASSWORD")
		if dbPassword == "" {
			dbPassword = "changeme"
		}
		dbName := os.Getenv("DB_NAME")
		if dbName == "" {
			dbName = "psyche_assessment"
		}
		dbSSLMode := os.Getenv("DB_SSLMODE")
		if dbSSLMode == "" {
			dbSSLMode = "disable"
		}
		dbDSN = fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=Asia/Jakarta",
			dbHost, dbUser, dbPassword, dbName, dbPort, dbSSLMode)
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisHost := os.Getenv("REDIS_HOST")
		if redisHost == "" {
			redisHost = "localhost"
		}
		redisPort := os.Getenv("REDIS_PORT")
		if redisPort == "" {
			redisPort = "6379"
		}
		redisAddr = fmt.Sprintf("%s:%s", redisHost, redisPort)
	}
	redisPassword := os.Getenv("REDIS_PASSWORD")

	redisDBStr := os.Getenv("REDIS_DB")
	redisDB, _ := strconv.Atoi(redisDBStr) // default 0

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "your-persona-super-secret-key-change-in-production-123456"
	}

	// Object storage (MinIO dev / R2 prod) — result PDF signed-URL downloads.
	s3Endpoint := os.Getenv("S3_ENDPOINT")
	if s3Endpoint == "" {
		s3Endpoint = "http://localhost:9000"
	}
	s3Region := os.Getenv("S3_REGION")
	if s3Region == "" {
		s3Region = "auto"
	}
	s3Bucket := os.Getenv("S3_BUCKET")
	if s3Bucket == "" {
		s3Bucket = "your-personas-reports"
	}
	s3AccessKey := os.Getenv("S3_ACCESS_KEY")
	if s3AccessKey == "" {
		s3AccessKey = "minioadmin"
	}
	s3SecretKey := os.Getenv("S3_SECRET_KEY")
	if s3SecretKey == "" {
		s3SecretKey = "minioadmin"
	}
	s3UsePathStyleStr := os.Getenv("S3_USE_PATH_STYLE")
	if s3UsePathStyleStr == "" {
		s3UsePathStyleStr = "true"
	}
	s3UsePathStyle, _ := strconv.ParseBool(s3UsePathStyleStr)

	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "development"
	}
	logInstance := logger.NewLogger(appEnv)
	isProduction := appEnv == "production"
	turnstileSecretKey := os.Getenv("TURNSTILE_SECRET_KEY")
	allowedOrigins := os.Getenv("ALLOWED_ORIGINS")

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
		logInstance,
	)
	if err != nil {
		logInstance.Error("failed to initialize API", "error", err)
		os.Exit(1)
	}

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

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
