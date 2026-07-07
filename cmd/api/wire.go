//go:build wireinject
// +build wireinject

package main

import (
	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment"
	"github.com/aprxty3/your_persona_controller.git/internal/application/auth"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/cache/redis"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/gemini"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	asynqclient "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/queue/asynq"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/stubs"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/handler"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"github.com/google/wire"
	"github.com/hibiken/asynq"
	"github.com/labstack/echo/v4"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Wrapper providers to resolve concrete types using the typed aliases.
func provideGeminiClient(key GeminiAPIKey, model GeminiModel, maxConcurrent int64) (*gemini.Client, error) {
	return gemini.NewClient(string(key), string(model), maxConcurrent)
}

func providePostgresDB(dsn DBDSN) (*gorm.DB, error) {
	return postgres.NewPostgresDB(string(dsn))
}

func provideRedisClient(addr RedisAddr, password RedisPassword, db int) (*goredis.Client, error) {
	return redis.NewRedisClient(string(addr), string(password), db)
}

func provideAsynqClient(addr RedisAddr, password RedisPassword, db int) (*asynq.Client, error) {
	return asynqclient.NewAsynqClient(string(addr), string(password), db)
}

func provideJWTService(secret JWTSecret) *jwtservice.JWTService {
	return jwtservice.NewJWTService(string(secret))
}

// InitializeAPI wires up the entire application and returns the Echo router.
func InitializeAPI(
	geminiAPIKey GeminiAPIKey,
	geminiModel GeminiModel,
	maxConcurrent int64,
	dbDSN DBDSN,
	redisAddr RedisAddr,
	redisPassword RedisPassword,
	redisDB int,
	jwtSecret JWTSecret,
	loggerInstance logger.Logger,
) (*echo.Echo, error) {
	wire.Build(
		// ---------------------------------------------------------
		// Infrastructure Providers
		// ---------------------------------------------------------
		provideGeminiClient,
		wire.Bind(new(assessment.AIGeneratorService), new(*gemini.Client)),

		providePostgresDB,
		provideRedisClient,
		provideAsynqClient,
		provideJWTService,
		taskqueue.NewAsynqDispatcher,
		auth.NewNoopBreachChecker,

		// ---------------------------------------------------------
		// Repositories
		// ---------------------------------------------------------
		// Postgres Repositories
		postgres.NewUserRepository,
		postgres.NewGuestSessionRepository,
		postgres.NewVerificationTokenRepository,

		// Stubs for assessment interfaces
		stubs.NewStubTestResultRepository,
		stubs.NewStubAnswerRepository,
		stubs.NewStubDistributedLockService,
		stubs.NewStubIdempotencyService,
		stubs.NewStubPDFQueueService,

		// Redis Services
		redis.NewOTPRateLimitService,

		// ---------------------------------------------------------
		// Application (Usecase) Providers
		// ---------------------------------------------------------
		assessment.NewSubmitAssessmentUseCase,
		auth.NewCreateGuestSessionUseCase,
		auth.NewRegisterUseCase,
		auth.NewVerifyEmailOTPUseCase,
		auth.NewResendEmailOTPUseCase,
		auth.NewLoginUseCase,

		// ---------------------------------------------------------
		// Delivery (HTTP) Providers
		// ---------------------------------------------------------
		handler.NewAssessmentHandler,
		handler.NewAuthHandler,
		http.SetupRouter,
	)
	return nil, nil
}
