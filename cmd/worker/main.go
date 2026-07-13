package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	appdeletion "github.com/aprxty3/your_persona_controller.git/internal/application/deletionrequest"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/mailer"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	pgaccount "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/account"
	pgdeletionrequest "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/deletionrequest"
	pgtestresult "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/testresult"
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

	smtpMailer := mailer.NewSMTPMailer(
		envOr("SMTP_HOST", "localhost"), smtpPort,
		os.Getenv("SMTP_USER"), os.Getenv("SMTP_PASSWORD"),
		envOr("SMTP_FROM", "noreply@yourpersonas.com"),
	)
	emailHandler := workerhandler.NewEmailHandler(smtpMailer, logInstance)

	anonymizeUseCase := appdeletion.NewAnonymizeUseCase(
		db,
		pgdeletionrequest.NewRepository(db, logInstance),
		pgaccount.NewUserRepository(db, logInstance),
		pgaccount.NewGuestSessionRepository(db, logInstance),
		pgtestresult.NewTestResultRepository(db, logInstance),
		s3Client,
		dispatcher,
		logInstance,
	)
	anonymizeHandler := workerhandler.NewAnonymizeHandler(anonymizeUseCase, logInstance)

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
		},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc(taskqueue.TaskSendEmail, emailHandler.ProcessTask)
	mux.HandleFunc(taskqueue.TaskDeletionScan, anonymizeHandler.ProcessScan)
	mux.HandleFunc(taskqueue.TaskAnonymize, anonymizeHandler.ProcessAnonymize)

	// Scheduler: hourly grace-period scan (a day-scale deadline doesn't need a tighter tick).
	scheduler := asynq.NewScheduler(redisOpt, nil)
	if _, err := scheduler.Register(
		"@every 1h",
		asynq.NewTask(taskqueue.TaskDeletionScan, nil),
		asynq.Queue(taskqueue.QueueLow),
	); err != nil {
		log.Fatalf("Failed to register deletion scan schedule: %v", err)
	}
	if err := scheduler.Start(); err != nil {
		log.Fatalf("Failed to start scheduler: %v", err)
	}

	// Kick one scan immediately at boot
	if _, err := asynqClient.EnqueueContext(context.Background(),
		asynq.NewTask(taskqueue.TaskDeletionScan, nil), asynq.Queue(taskqueue.QueueLow)); err != nil {
		log.Printf("WARN: failed to enqueue startup deletion scan (next hourly tick will cover it): %v", err)
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
