package main

import (
	"log"
	"os"

	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
)

func main() {
	dbDSN := os.Getenv("DB_DSN")
	if dbDSN == "" {
		dbDSN = "host=localhost user=postgres password=postgres dbname=your_persona port=5432 sslmode=disable TimeZone=Asia/Jakarta"
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
	)
	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	log.Println("Migration completed successfully!")
}
