package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"
)

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	redisPassword := os.Getenv("REDIS_PASSWORD")

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

	// Register dummy handlers so we don't panic on startup
	mux.HandleFunc("send:email", func(ctx context.Context, t *asynq.Task) error {
		log.Printf("Worker executing task %s with payload %s", t.Type(), string(t.Payload()))
		return nil
	})

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
