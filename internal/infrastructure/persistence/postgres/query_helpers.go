package postgres

import (
	"errors"

	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
)

// IsNotFound reports whether err is GORM's "record not found" sentinel — the
// single check every FindByX repository method in this codebase uses to
// translate "no matching row" into the (nil, nil) convention, rather than
// treating it as a query failure.
func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// LogQueryError logs a failed query under a consistent shape ("query failed",
// op, error) and returns err unchanged. It's the one-line replacement for the
// `if err != nil { log.Error(...); return err }` boilerplate that ends nearly
// every repository method (Create/Update/Delete/Count/...) across this
// codebase's persistence layer — callers still build and run the GORM query
// however they need to; only the error-logging tail is shared.
func LogQueryError(log logger.Logger, op string, err error) error {
	if err != nil {
		log.Error("query failed", "op", op, "error", err)
	}
	return err
}
