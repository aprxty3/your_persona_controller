package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	echo "github.com/labstack/echo/v4"
)

// Typed aliases to avoid Wire's type ambiguity with multiple string arguments.
type GeminiAPIKey string
type GeminiModel string
type DBDSN string
type RedisAddr string
type RedisPassword string
type JWTSecret string

// @title Your Persona API
// @version 1.0
// @description API Server for Your Persona psychological assessment platform.
// @host localhost:8080
// @BasePath /
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
	)
	if err != nil {
		log.Fatalf("Failed to initialize API: %v", err)
	}

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	// ---------------------------------------------------------
	// SERVER START & GRACEFUL SHUTDOWN
	// ---------------------------------------------------------
	go func() {
		log.Printf("Server is starting on port %s...", port)
		if err := app.Start(":" + port); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server forced to shutdown abruptly: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)

	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Block the main thread until a signal is received
	<-quit
	log.Println("Interrupt signal received. Shutting down server gracefully...")

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
		log.Fatalf("Server shutdown failed or timed out: %v", err)
	}

	log.Println("Server exited properly.")
}
