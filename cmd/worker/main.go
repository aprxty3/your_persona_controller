package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/mailer"
	workerhandler "github.com/aprxty3/your_persona_controller.git/internal/interfaces/worker"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/hibiken/asynq"
)

func main() {
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

	smtpHost := os.Getenv("SMTP_HOST")
	if smtpHost == "" {
		smtpHost = "localhost"
	}
	smtpPort, err := strconv.Atoi(os.Getenv("SMTP_PORT"))
	if err != nil || smtpPort == 0 {
		smtpPort = 1025
	}
	smtpUser := os.Getenv("SMTP_USER")
	smtpPassword := os.Getenv("SMTP_PASSWORD")
	smtpFrom := os.Getenv("SMTP_FROM")
	if smtpFrom == "" {
		smtpFrom = "noreply@yourpersonas.com"
	}

	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "development"
	}
	logInstance := logger.NewLogger(appEnv)

	smtpMailer := mailer.NewSMTPMailer(smtpHost, smtpPort, smtpUser, smtpPassword, smtpFrom)
	emailHandler := workerhandler.NewEmailHandler(smtpMailer, logInstance)

	log.Println("Starting background worker...")
	srv := asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     redisAddr,
			Password: redisPassword,
		},
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
	mux.HandleFunc("send:email", emailHandler.ProcessTask)

	go func() {
		if err := srv.Run(mux); err != nil {
			log.Fatalf("Worker server crash: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down worker gracefully...")
	srv.Shutdown()
	log.Println("Worker stopped.")
}
