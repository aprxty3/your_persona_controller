package main

import (
	"context"
	"errors"
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
		dbDSN = "host=localhost user=postgres password=postgres dbname=your_persona port=5432 sslmode=disable TimeZone=Asia/Jakarta"
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
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
