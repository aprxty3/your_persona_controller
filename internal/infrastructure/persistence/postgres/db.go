package postgres

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewPostgresDB creates and returns a *gorm.DB connected to PostgreSQL.
// DSN format: "host=... user=... password=... dbname=... port=5432 sslmode=disable TimeZone=Asia/Jakarta"
// Migration is intentionally NOT called here — run `go run ./cmd/migrate` manually.
func NewPostgresDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to open connection: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to get sql.DB: %w", err)
	}

	// Basic connection pool settings — tune via env in production.
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)

	return db, nil
}
