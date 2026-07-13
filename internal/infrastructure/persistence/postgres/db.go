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
	// IgnoreRecordNotFoundError: every repository already treats "no row" as a
	// normal outcome (returns nil, nil) — logging it as a warning on every
	// lookup miss (email not registered yet, referral code not generated yet,
	// etc.) is just noise, not a real error signal.
	gormLogger := logger.New(stdlog.New(os.Stdout, "\r\n", stdlog.LstdFlags), logger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  true,
	})

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
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
