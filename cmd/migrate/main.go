package main

import (
	"fmt"
	"log"
	"os"

	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
)

func main() {
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

	log.Println("Connecting to database for migration...")
	db, err := postgres.NewPostgresDB(dbDSN)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	log.Println("Running GORM AutoMigrate...")
	err = db.AutoMigrate(
		&postgres.UserModel{},
		&postgres.GuestSessionModel{},
		&postgres.TestResultModel{},
		&postgres.VerificationTokenModel{},
		&postgres.ReferralCodeModel{},
		&postgres.ReferralEventModel{},
		&postgres.DataDeletionRequestModel{},
		&postgres.QuestionModel{},
		&postgres.QuestionTranslationModel{},
		&postgres.AnswerModel{},
		&postgres.InsightTemplateModel{},
		&postgres.PromptAuditLogModel{},
	)
	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	log.Println("Migration completed successfully!")
}
