package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/aprxty3/your_persona_controller.git/internal/application/auditpurge"
	appdeletion "github.com/aprxty3/your_persona_controller.git/internal/application/deletionrequest"
	"github.com/aprxty3/your_persona_controller.git/internal/application/guestpurge"
	apppdf "github.com/aprxty3/your_persona_controller.git/internal/application/pdf"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/i18n"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/mailer"
	pdfrenderer "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/pdf"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	pgaccount "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/account"
	pgassessment "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/assessment"
	pgdeletionrequest "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/deletionrequest"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/storage/s3"
	workerhandler "github.com/aprxty3/your_persona_controller.git/internal/interfaces/worker"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"github.com/hibiken/asynq"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = fmt.Sprintf("%s:%s", envOr("REDIS_HOST", "localhost"), envOr("REDIS_PORT", "6379"))
	}
	redisOpt := asynq.RedisClientOpt{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
	}

	smtpPort, err := strconv.Atoi(os.Getenv("SMTP_PORT"))
	if err != nil || smtpPort == 0 {
		smtpPort = 1025
	}

	appEnv := envOr("APP_ENV", "development")
	logInstance := logger.NewLogger(appEnv)

	// Postgres — the anonymize worker mutates users/test_results/guest_sessions.
	dbDSN := os.Getenv("DB_DSN")
	if dbDSN == "" {
		dbDSN = fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=Asia/Jakarta",
			envOr("DB_HOST", "localhost"), envOr("DB_USER", "postgres"), envOr("DB_PASSWORD", "changeme"),
			envOr("DB_NAME", "psyche_assessment"), envOr("DB_PORT", "5432"), envOr("DB_SSLMODE", "disable"))
	}
	db, err := postgres.NewPostgresDB(dbDSN)
	if err != nil {
		log.Fatalf("Worker failed to connect to database: %v", err)
	}

	// Object storage (MinIO dev / R2 prod) — anonymization must delete PDFs
	usePathStyle, _ := strconv.ParseBool(envOr("S3_USE_PATH_STYLE", "true"))
	s3Client, err := s3.NewClient(
		envOr("S3_ENDPOINT", "http://localhost:9000"),
		envOr("S3_REGION", "auto"),
		envOr("S3_BUCKET", "your-personas-reports"),
		envOr("S3_ACCESS_KEY", "minioadmin"),
		envOr("S3_SECRET_KEY", "minioadmin"),
		usePathStyle,
	)
	if err != nil {
		log.Fatalf("Worker failed to init object storage client: %v", err)
	}

	// Asynq client — the worker also PRODUCES tasks
	asynqClient := asynq.NewClient(redisOpt)
	defer asynqClient.Close()
	dispatcher := taskqueue.NewAsynqDispatcher(asynqClient)

	i18nCatalog, err := i18n.LoadCatalog()
	if err != nil {
		log.Fatalf("Worker failed to load i18n catalog: %v", err)
	}

	smtpMailer := mailer.NewSMTPMailer(
		envOr("SMTP_HOST", "localhost"), smtpPort,
		os.Getenv("SMTP_USER"), os.Getenv("SMTP_PASSWORD"),
		envOr("SMTP_FROM", "noreply@yourpersonas.com"),
		i18nCatalog,
	)
	emailHandler := workerhandler.NewEmailHandler(smtpMailer, logInstance)

	// Shared repos — reused across use cases so each doesn't reopen its own
	// db-bound instance (tx-scoped writes still construct their own via
	// pg*.NewXRepository(tx, log) inside db.Transaction, per this repo's convention).
	testResultRepo := pgassessment.NewTestResultRepository(db, logInstance)
	guestSessionRepo := pgaccount.NewGuestSessionRepository(db, logInstance)

	anonymizeUseCase := appdeletion.NewAnonymizeUseCase(
		db,
		pgdeletionrequest.NewRepository(db, logInstance),
		pgaccount.NewUserRepository(db, logInstance),
		guestSessionRepo,
		testResultRepo,
		s3Client,
		dispatcher,
		logInstance,
	)
	anonymizeHandler := workerhandler.NewAnonymizeHandler(anonymizeUseCase, logInstance)

	purgeUseCase := guestpurge.NewPurgeGuestTTLUseCase(db, testResultRepo, guestSessionRepo, s3Client, logInstance)
	auditPurgeUseCase := auditpurge.NewPurgeAuditTTLUseCase(pgassessment.NewPromptAuditLogRepository(db, logInstance), logInstance)
	purgeHandler := workerhandler.NewPurgeHandler(purgeUseCase, auditPurgeUseCase, logInstance)

	generatePDFUseCase := apppdf.NewGeneratePDFUseCase(
		testResultRepo,
		pgassessment.NewAnswerRepository(db, logInstance),
		pgassessment.NewQuestionRepository(db, logInstance),
		pgassessment.NewInsightTemplateRepository(db, logInstance),
		pgaccount.NewUserRepository(db, logInstance),
		guestSessionRepo,
		pdfrenderer.NewMarotoRenderer(),
		s3Client,
		logInstance,
	)
	pdfHandler := workerhandler.NewPDFHandler(generatePDFUseCase, logInstance)

	log.Println("Starting background worker...")
	srv := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Concurrency: 10,
			Queues: map[string]int{
				"critical": 6,
				"default":  3,
				"pdf":      2,
				"low":      1,
			},
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
				if task.Type() != taskqueue.TaskGeneratePDF {
					return
				}
				retried, _ := asynq.GetRetryCount(ctx)
				maxRetry, _ := asynq.GetMaxRetry(ctx)
				if retried < maxRetry {
					return
				}
				var payload taskqueue.GeneratePDFPayload
				if jsonErr := json.Unmarshal(task.Payload(), &payload); jsonErr != nil {
					logInstance.Error("pdf worker: failed to unmarshal payload on final failure", "error", jsonErr)
					return
				}
				if updErr := testResultRepo.UpdatePDFStatus(context.Background(), payload.TestResultID, nil, testresult.PDFStatusFailed); updErr != nil {
					logInstance.Error("pdf worker: failed to mark pdf_status=failed after max retries", "test_result_id", payload.TestResultID, "error", updErr)
				}
			}),
		},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc(taskqueue.TaskSendEmail, emailHandler.ProcessTask)
	mux.HandleFunc(taskqueue.TaskDeletionScan, anonymizeHandler.ProcessScan)
	mux.HandleFunc(taskqueue.TaskAnonymize, anonymizeHandler.ProcessAnonymize)
	mux.HandleFunc(taskqueue.TaskPurgeGuest, purgeHandler.ProcessPurge)
	mux.HandleFunc(taskqueue.TaskPurgeAuditLog, purgeHandler.ProcessAuditPurge)
	mux.HandleFunc(taskqueue.TaskGeneratePDF, pdfHandler.ProcessTask)

	scheduler := asynq.NewScheduler(redisOpt, nil)
	if _, err := scheduler.Register(
		"@every 1h",
		asynq.NewTask(taskqueue.TaskDeletionScan, nil),
		asynq.Queue(taskqueue.QueueLow),
	); err != nil {
		log.Fatalf("Failed to register deletion scan schedule: %v", err)
	}
	if _, err := scheduler.Register(
		"@daily",
		asynq.NewTask(taskqueue.TaskPurgeGuest, nil),
		asynq.Queue(taskqueue.QueueLow),
	); err != nil {
		log.Fatalf("Failed to register guest ttl purge schedule: %v", err)
	}
	if _, err := scheduler.Register(
		"@daily",
		asynq.NewTask(taskqueue.TaskPurgeAuditLog, nil),
		asynq.Queue(taskqueue.QueueLow),
	); err != nil {
		log.Fatalf("Failed to register audit ttl purge schedule: %v", err)
	}
	if err := scheduler.Start(); err != nil {
		log.Fatalf("Failed to start scheduler: %v", err)
	}

	// Kick one scan immediately at boot
	if _, err := asynqClient.EnqueueContext(context.Background(),
		asynq.NewTask(taskqueue.TaskDeletionScan, nil), asynq.Queue(taskqueue.QueueLow)); err != nil {
		log.Printf("WARN: failed to enqueue startup deletion scan (next hourly tick will cover it): %v", err)
	}
	if _, err := asynqClient.EnqueueContext(context.Background(),
		asynq.NewTask(taskqueue.TaskPurgeGuest, nil), asynq.Queue(taskqueue.QueueLow)); err != nil {
		log.Printf("WARN: failed to enqueue startup guest ttl purge (next daily tick will cover it): %v", err)
	}
	if _, err := asynqClient.EnqueueContext(context.Background(),
		asynq.NewTask(taskqueue.TaskPurgeAuditLog, nil), asynq.Queue(taskqueue.QueueLow)); err != nil {
		log.Printf("WARN: failed to enqueue startup audit ttl purge (next daily tick will cover it): %v", err)
	}

	go func() {
		if err := srv.Run(mux); err != nil {
			log.Fatalf("Worker server crash: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down worker gracefully...")
	scheduler.Shutdown()
	srv.Shutdown()
	log.Println("Worker stopped.")
}
