// Package postgres provides the GORM connection setup, table models, and
// query helpers shared by every domain-specific repository package beneath it
// (account, assessment, deletionrequest).
package postgres

import (
	"fmt"
	stdlog "log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewPostgresDB creates and returns a *gorm.DB connected to PostgreSQL.
func NewPostgresDB(dsn string) (*gorm.DB, error) {
	gormLogger := logger.New(stdlog.New(os.Stdout, "\r\n", stdlog.LstdFlags), logger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  true,
	})

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:         gormLogger,
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to open connection: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to get sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)

	return db, nil
}
