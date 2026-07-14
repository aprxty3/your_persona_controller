//go:build wireinject
// +build wireinject

package main

import (
	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment"
	"github.com/aprxty3/your_persona_controller.git/internal/application/auth"
	"github.com/aprxty3/your_persona_controller.git/internal/application/dashboard"
	"github.com/aprxty3/your_persona_controller.git/internal/application/deletionrequest"
	"github.com/aprxty3/your_persona_controller.git/internal/application/profile"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/cache/redis"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/gemini"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	pgaccount "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/account"
	pgassessment "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/assessment"
	pgdeletionrequest "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/deletionrequest"
	asynqclient "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/queue/asynq"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/security"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/storage/s3"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/handler"
	appmiddleware "github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/middleware"
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

func provideS3Client(endpoint S3Endpoint, region S3Region, bucket S3Bucket, accessKey S3AccessKey, secretKey S3SecretKey, usePathStyle S3UsePathStyle) (*s3.Client, error) {
	return s3.NewClient(string(endpoint), string(region), string(bucket), string(accessKey), string(secretKey), bool(usePathStyle))
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
	s3Endpoint S3Endpoint,
	s3Region S3Region,
	s3Bucket S3Bucket,
	s3AccessKey S3AccessKey,
	s3SecretKey S3SecretKey,
	s3UsePathStyle S3UsePathStyle,
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
		security.NewHIBPBreachChecker,

		provideS3Client,
		wire.Bind(new(assessment.PDFSignerService), new(*s3.Client)),

		// ---------------------------------------------------------
		// Repositories
		// ---------------------------------------------------------
		// Postgres Repositories
		pgaccount.NewUserRepository,
		pgaccount.NewGuestSessionRepository,
		pgaccount.NewVerificationTokenRepository,
		pgaccount.NewReferralRepository,
		pgdeletionrequest.NewRepository,
		pgassessment.NewTestResultRepository,
		pgassessment.NewQuestionRepository,

		wire.Bind(new(assessment.TestResultRepository), new(testresult.TestResultRepository)),
		wire.Bind(new(assessment.QuestionRepository), new(*pgassessment.QuestionRepository)),
		wire.Bind(new(assessment.ResultRepository), new(testresult.TestResultRepository)),
		wire.Bind(new(assessment.QuestionCatalogRepository), new(*pgassessment.QuestionRepository)),
		wire.Bind(new(dashboard.TestResultRepository), new(testresult.TestResultRepository)),

		asynqclient.NewPDFQueueService,

		// Redis Services
		redis.NewOTPRateLimitService,
		redis.NewIPRateLimitService,
		redis.NewTokenStore,
		redis.NewDistributedLockService,
		redis.NewIdempotencyService,

		// ---------------------------------------------------------
		// Application (Usecase) Providers
		// ---------------------------------------------------------
		assessment.NewSubmitAssessmentUseCase,
		assessment.NewResultUseCase,
		assessment.NewQuestionCatalogUseCase,
		dashboard.NewDashboardUseCase,
		auth.NewCreateGuestSessionUseCase,
		auth.NewRegisterUseCase,
		auth.NewAccountUseCase,
		auth.NewSessionUseCase,
		profile.NewProfileUseCase,
		deletionrequest.NewDeletionUseCase,

		// ---------------------------------------------------------
		// Delivery (HTTP) Providers
		// ---------------------------------------------------------
		appmiddleware.NewAuthMiddleware,
		appmiddleware.NewLocaleMiddleware,
		handler.NewAssessmentHandler,
		handler.NewResultHandler,
		handler.NewDashboardHandler,
		handler.NewAuthHandler,
		handler.NewAccountHandler,
		handler.NewHealthHandler,
		http.SetupRouter,
	)
	return nil, nil
}
