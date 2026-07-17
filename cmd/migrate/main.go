package main

import (
	"log"

	"github.com/aprxty3/your_persona_controller.git/internal/config"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
)

func main() {
	log.Println("Connecting to database for migration...")
	db, err := postgres.NewPostgresDB(config.PostgresDSN())
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
