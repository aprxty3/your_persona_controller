// Package guestpurge implements the scheduled purge of expired guest-owned data.
package guestpurge

import (
	"context"
	"fmt"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	pgassessment "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/assessment"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
)

// PDFStorage is the port the purge job uses to delete R2/MinIO objects.
type PDFStorage interface {
	DeleteByURL(ctx context.Context, rawURL string) error
}

// PurgeGuestTTLUseCase implements the daily Guest data retention sweep
// Expired Guest test results have their R2 PDF deleted first, then their DB rows (TestResult, Answer, PromptAuditLog),
// then the owning GuestSession itself.
type PurgeGuestTTLUseCase struct {
	db             *gorm.DB
	testResultRepo testresult.Repository
	guestRepo      account.GuestSessionRepository
	pdfStorage     PDFStorage
	log            logger.Logger
}

// NewPurgeGuestTTLUseCase creates a new PurgeGuestTTLUseCase.
func NewPurgeGuestTTLUseCase(
	db *gorm.DB,
	testResultRepo testresult.Repository,
	guestRepo account.GuestSessionRepository,
	pdfStorage PDFStorage,
	log logger.Logger,
) *PurgeGuestTTLUseCase {
	return &PurgeGuestTTLUseCase{
		db:             db,
		testResultRepo: testResultRepo,
		guestRepo:      guestRepo,
		pdfStorage:     pdfStorage,
		log:            log.With("usecase", "guest_ttl_purge"),
	}
}

// Execute purges every Guest-owned test result whose expires_at has passed.
// Failures on an individual row are logged and skipped rather than aborting
// the whole run — tomorrow's tick retries it, and re-deleting an R2 object
// that's already gone is a no-op success, so partial failure is safe by
// design.
func (uc *PurgeGuestTTLUseCase) Execute(ctx context.Context) (int, error) {
	expired, err := uc.testResultRepo.FindExpiredGuestResults(ctx)
	if err != nil {
		return 0, fmt.Errorf("guest_ttl_purge: find expired: %w", err)
	}

	purged := 0
	expiredSessionIDs := make(map[string]struct{})

	for i := range expired {
		r := &expired[i]

		if r.PDFUrl != nil {
			if err := uc.pdfStorage.DeleteByURL(ctx, *r.PDFUrl); err != nil {
				uc.log.Error("guest ttl purge failed", "step", "delete_pdf", "test_result_id", r.ID, "error", err)
				continue
			}
		}

		txErr := uc.db.Transaction(func(tx *gorm.DB) error {
			if err := pgassessment.NewAnswerRepository(tx, uc.log).DeleteByTestResultID(ctx, r.ID); err != nil {
				return fmt.Errorf("tx: delete answers: %w", err)
			}
			if err := pgassessment.NewPromptAuditLogRepository(tx, uc.log).DeleteByTestResultID(ctx, r.ID); err != nil {
				return fmt.Errorf("tx: delete prompt audit logs: %w", err)
			}
			if err := pgassessment.NewTestResultRepository(tx, uc.log).DeleteByID(ctx, r.ID); err != nil {
				return fmt.Errorf("tx: delete test result: %w", err)
			}
			return nil
		})
		if txErr != nil {
			uc.log.Error("guest ttl purge failed", "step", "delete_row_transaction", "test_result_id", r.ID, "error", txErr)
			continue
		}

		purged++
		if r.GuestSessionID != nil {
			expiredSessionIDs[*r.GuestSessionID] = struct{}{}
		}
	}

	for sessionID := range expiredSessionIDs {
		if err := uc.guestRepo.DeleteBySessionID(ctx, sessionID); err != nil {
			uc.log.Error("guest ttl purge failed", "step", "delete_guest_session", "session_id", sessionID, "error", err)
		}
	}

	orphansPurged := uc.purgeOrphanSessions(ctx)

	if purged > 0 || len(expiredSessionIDs) > 0 || orphansPurged > 0 {
		uc.log.Info("guest ttl purge done", "results_purged", purged, "guest_sessions_purged", len(expiredSessionIDs), "orphan_sessions_purged", orphansPurged)
	}
	return purged, nil
}

// purgeOrphanSessions sweeps expired, unclaimed guest sessions that never
// produced a test result. FindExpiredUnclaimed's query excludes
// any session that still has a test_results row, so sessions whose result is
// alive — or pending deletion by the result loop above — are never touched.
// Per-row skip-on-error, same policy as the rest of this job.
func (uc *PurgeGuestTTLUseCase) purgeOrphanSessions(ctx context.Context) int {
	orphans, err := uc.guestRepo.FindExpiredUnclaimed(ctx)
	if err != nil {
		uc.log.Error("guest ttl purge failed", "step", "find_orphan_sessions", "error", err)
		return 0
	}

	purged := 0
	for _, s := range orphans {
		if err := uc.guestRepo.DeleteBySessionID(ctx, s.SessionID); err != nil {
			uc.log.Error("guest ttl purge failed", "step", "delete_orphan_session", "session_id", s.SessionID, "error", err)
			continue
		}
		purged++
	}
	return purged
}
