//go:build wireinject
// +build wireinject

package main

import (
	"context"

	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment"
	assessmentdto "github.com/aprxty3/your_persona_controller.git/internal/application/assessment/dto"
	"github.com/aprxty3/your_persona_controller.git/internal/application/auth"
	"github.com/aprxty3/your_persona_controller.git/internal/application/user_dashboard"
	"github.com/aprxty3/your_persona_controller.git/internal/application/deletionrequest"
	"github.com/aprxty3/your_persona_controller.git/internal/application/profile"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/content"
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

func provideTurnstileVerifier(secretKey TurnstileSecretKey, log logger.Logger) auth.TurnstileVerifier {
	return security.NewTurnstileVerifier(string(secretKey), log)
}

// provideIsProduction unwraps the typed alias into the plain bool that
// router.go / auth_handler.go actually consume — those HTTP-layer files stay
// Wire-agnostic and just take a bool, matching ordinary Go convention.
func provideIsProduction(v IsProduction) bool {
	return bool(v)
}

func provideAllowedOrigins(v AllowedOrigins) []string {
	return http.ParseAllowedOrigins(string(v))
}

func provideIPExtractor(v TrustedProxies) (echo.IPExtractor, error) {
	return http.ParseTrustedProxies(string(v))
}

// assessmentIPRateLimiterAdapter bridges *redis.IPRateLimitService to assessment.IPRateLimiter.
type assessmentIPRateLimiterAdapter struct {
	svc *redis.IPRateLimitService
}

func (a *assessmentIPRateLimiterAdapter) Allow(ctx context.Context, scope assessmentdto.IPRateLimitScope, ip string) (bool, int, error) {
	return a.svc.Allow(ctx, redis.IPScope(scope), ip)
}

func provideAssessmentIPRateLimiter(svc *redis.IPRateLimitService) assessment.IPRateLimiter {
	return &assessmentIPRateLimiterAdapter{svc: svc}
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
	turnstileSecretKey TurnstileSecretKey,
	isProduction IsProduction,
	allowedOrigins AllowedOrigins,
	trustedProxies TrustedProxies,
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
		provideTurnstileVerifier,
		provideIsProduction,
		provideAllowedOrigins,
		provideIPExtractor,

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
		pgassessment.NewInsightTemplateRepository,

		wire.Bind(new(assessment.TestResultRepository), new(testresult.TestResultRepository)),
		wire.Bind(new(assessment.QuestionRepository), new(*pgassessment.QuestionRepository)),
		wire.Bind(new(assessment.ResultRepository), new(testresult.TestResultRepository)),
		wire.Bind(new(assessment.QuestionCatalogRepository), new(*pgassessment.QuestionRepository)),
		wire.Bind(new(dashboard.TestResultRepository), new(testresult.TestResultRepository)),
		wire.Bind(new(dashboard.InsightTemplateRepository), new(content.InsightTemplateRepository)),

		asynqclient.NewPDFQueueService,

		// Redis Services
		redis.NewOTPRateLimitService,
		wire.Bind(new(auth.OTPRateLimiter), new(*redis.OTPRateLimitService)),
		redis.NewIPRateLimitService,
		wire.Bind(new(auth.IPRateLimiter), new(*redis.IPRateLimitService)),
		provideAssessmentIPRateLimiter,
		redis.NewTokenStore,
		wire.Bind(new(auth.SessionTokenStore), new(*redis.TokenStore)),
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
