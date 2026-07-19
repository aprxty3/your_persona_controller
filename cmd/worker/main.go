// Command worker runs the Asynq background worker: email, PDF generation, anonymization, and TTL purge jobs.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/aprxty3/your_persona_controller.git/internal/application/auditpurge"
	appdeletion "github.com/aprxty3/your_persona_controller.git/internal/application/deletionrequest"
	"github.com/aprxty3/your_persona_controller.git/internal/application/guestpurge"
	apppdf "github.com/aprxty3/your_persona_controller.git/internal/application/pdf"
	"github.com/aprxty3/your_persona_controller.git/internal/config"
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

func main() {
	redisOpt := asynq.RedisClientOpt{
		Addr:     config.RedisAddr(),
		Password: os.Getenv("REDIS_PASSWORD"),
	}

	smtpPort := config.EnvInt("SMTP_PORT", 1025)

	appEnv := config.EnvOr("APP_ENV", "development")
	logInstance := logger.NewLogger(appEnv)
	isProduction := appEnv == "production"

	dbPassword := config.EnvOr("DB_PASSWORD", "changeme")
	s3AccessKey := config.EnvOr("S3_ACCESS_KEY", "minioadmin")
	s3SecretKey := config.EnvOr("S3_SECRET_KEY", "minioadmin")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPassword := os.Getenv("SMTP_PASSWORD")

	// same production gate as cmd/api — a worker silently anonymizing
	// users or emailing OTPs through dev fallback credentials is exactly the
	// kind of incident this is meant to catch before it ships.
	if isProduction {
		config.RequireProduction(logInstance,
			config.Check{Name: "DB_PASSWORD", Value: dbPassword, InsecureDefault: "changeme"},
			config.Check{Name: "S3_ACCESS_KEY", Value: s3AccessKey, InsecureDefault: "minioadmin"},
			config.Check{Name: "S3_SECRET_KEY", Value: s3SecretKey, InsecureDefault: "minioadmin"},
			config.Check{Name: "SMTP_USER", Value: smtpUser},
			config.Check{Name: "SMTP_PASSWORD", Value: smtpPassword},
		)
	}

	// Postgres — the anonymize worker mutates users/test_results/guest_sessions.
	db, err := postgres.NewPostgresDB(config.PostgresDSN())
	if err != nil {
		log.Fatalf("Worker failed to connect to database: %v", err)
	}

	// Object storage (MinIO dev / R2 prod) — anonymization must delete PDFs
	usePathStyle, _ := strconv.ParseBool(config.EnvOr("S3_USE_PATH_STYLE", "true"))
	s3Client, err := s3.NewClient(
		config.EnvOr("S3_ENDPOINT", "http://localhost:9000"),
		config.EnvOr("S3_REGION", "auto"),
		config.EnvOr("S3_BUCKET", "your-personas-reports"),
		s3AccessKey,
		s3SecretKey,
		usePathStyle,
	)
	if err != nil {
		log.Fatalf("Worker failed to init object storage client: %v", err)
	}

	// Asynq client — the worker also PRODUCES tasks
	asynqClient := asynq.NewClient(redisOpt)
	defer func() { _ = asynqClient.Close() }()
	dispatcher := taskqueue.NewAsynqDispatcher(asynqClient)

	i18nCatalog, err := i18n.LoadCatalog()
	if err != nil {
		log.Fatalf("Worker failed to load i18n catalog: %v", err)
	}

	smtpMailer := mailer.NewSMTPMailer(
		config.EnvOr("SMTP_HOST", "localhost"), smtpPort,
		smtpUser, smtpPassword,
		config.EnvOr("SMTP_FROM", "noreply@yourpersonas.com"),
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

	// Two servers, split by workload class
	pdfSrv := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Concurrency: config.EnvInt("WORKER_PDF_CONCURRENCY", 2),
			Queues:      map[string]int{taskqueue.QueuePDF: 1},
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, _ error) {
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
				// This runs on the task's final-failure path — ctx may already be near
				// cancellation by the time asynq calls us, but the failure write still
				// has to land. WithoutCancel keeps ctx's values while dropping its
				// cancellation, same convention used for in-flight Gemini calls (AGENTS.md).
				if updErr := testResultRepo.UpdatePDFStatus(context.WithoutCancel(ctx), payload.TestResultID, nil, testresult.PDFStatusFailed); updErr != nil {
					logInstance.Error("pdf worker: failed to mark pdf_status=failed after max retries", "test_result_id", payload.TestResultID, "error", updErr)
				}
			}),
		},
	)
	pdfMux := asynq.NewServeMux()
	pdfMux.HandleFunc(taskqueue.TaskGeneratePDF, pdfHandler.ProcessTask)

	ioSrv := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Concurrency: config.EnvInt("WORKER_IO_CONCURRENCY", 8),
			Queues: map[string]int{
				taskqueue.QueueCritical: 6,
				taskqueue.QueueDefault:  3,
				taskqueue.QueueLow:      1,
			},
		},
	)
	ioMux := asynq.NewServeMux()
	ioMux.HandleFunc(taskqueue.TaskSendEmail, emailHandler.ProcessTask)
	ioMux.HandleFunc(taskqueue.TaskDeletionScan, anonymizeHandler.ProcessScan)
	ioMux.HandleFunc(taskqueue.TaskAnonymize, anonymizeHandler.ProcessAnonymize)
	ioMux.HandleFunc(taskqueue.TaskPurgeGuest, purgeHandler.ProcessPurge)
	ioMux.HandleFunc(taskqueue.TaskPurgeAuditLog, purgeHandler.ProcessAuditPurge)

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
		if err := pdfSrv.Run(pdfMux); err != nil {
			log.Fatalf("PDF worker server crash: %v", err)
		}
	}()
	go func() {
		if err := ioSrv.Run(ioMux); err != nil {
			log.Fatalf("IO worker server crash: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down worker gracefully...")
	scheduler.Shutdown()
	pdfSrv.Shutdown()
	ioSrv.Shutdown()
	log.Println("Worker stopped.")
}
