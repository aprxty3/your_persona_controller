package deletionrequest

import (
	"context"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/deletionrequest"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	pgaccount "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/account"
	pgassessment "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/assessment"
	pkglocale "github.com/aprxty3/your_persona_controller.git/pkg/locale"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"gorm.io/gorm"
)

// PDFStorage is the port the anonymization worker uses to delete R2/MinIO objects.
type PDFStorage interface {
	DeleteByURL(ctx context.Context, rawURL string) error
}

// AnonymizeRequest mirrors taskqueue.AnonymizeUserPayload on the consuming side.
type AnonymizeRequest struct {
	UserID            string
	DeletionRequestID string
}

// AnonymizeUseCase executes the UU PDP anonymization pipeline.
type AnonymizeUseCase struct {
	db             *gorm.DB
	deleteRepo     deletionrequest.Repository
	userRepo       account.UserRepository
	guestRepo      account.GuestSessionRepository
	testResultRepo testresult.TestResultRepository
	pdfStorage     PDFStorage
	dispatcher     taskqueue.Dispatcher
	log            logger.Logger
}

// NewAnonymizeUseCase creates a new AnonymizeUseCase.
func NewAnonymizeUseCase(
	db *gorm.DB,
	deleteRepo deletionrequest.Repository,
	userRepo account.UserRepository,
	guestRepo account.GuestSessionRepository,
	testResultRepo testresult.TestResultRepository,
	pdfStorage PDFStorage,
	dispatcher taskqueue.Dispatcher,
	log logger.Logger,
) *AnonymizeUseCase {
	return &AnonymizeUseCase{
		db:             db,
		deleteRepo:     deleteRepo,
		userRepo:       userRepo,
		guestRepo:      guestRepo,
		testResultRepo: testResultRepo,
		pdfStorage:     pdfStorage,
		dispatcher:     dispatcher,
		log:            log.With("usecase", "anonymize"),
	}
}

// ProcessExpired finds every request whose 14-day grace period has elapsed, enqueues an anonymize job for each.
func (uc *AnonymizeUseCase) ProcessExpired(ctx context.Context) (int, error) {
	expired, err := uc.deleteRepo.FindExpiredGracePeriod(ctx)
	if err != nil {
		return 0, fmt.Errorf("anonymize_scan: find expired: %w", err)
	}

	enqueued := 0
	for i := range expired {
		req := &expired[i]
		payload := taskqueue.AnonymizeUserPayload{
			UserID:            req.UserID,
			DeletionRequestID: req.ID,
		}
		if err := uc.dispatcher.EnqueueAnonymize(ctx, payload); err != nil {
			uc.log.Error("anonymize scan failed", "step", "enqueue", "request_id", req.ID, "error", err)
			continue
		}

		moved, err := uc.deleteRepo.TransitionStatus(ctx, req.ID, deletionrequest.StatusPendingGrace, deletionrequest.StatusProcessing, nil)
		if err != nil {
			uc.log.Error("anonymize scan failed", "step", "mark_processing", "request_id", req.ID, "error", err)
		} else if !moved {
			uc.log.Warn("anonymize scan skipped mark", "reason", "status_moved_concurrently", "request_id", req.ID)
		}
		enqueued++
	}

	if enqueued > 0 {
		uc.log.Info("anonymize scan done", "expired_found", len(expired), "jobs_enqueued", enqueued)
	}
	return enqueued, nil
}

// Anonymize is the worker-side executor for one anonymize:user job.
func (uc *AnonymizeUseCase) Anonymize(ctx context.Context, req AnonymizeRequest) error {
	dr, err := uc.deleteRepo.FindByID(ctx, req.DeletionRequestID)
	if err != nil {
		return fmt.Errorf("anonymize: find request: %w", err)
	}
	if dr == nil {
		uc.log.Warn("anonymize dropped", "reason", "request_not_found", "request_id", req.DeletionRequestID)
		return nil
	}

	switch dr.Status {
	case deletionrequest.StatusCancelled:
		uc.log.Info("anonymize dropped", "reason", "request_cancelled", "request_id", dr.ID)
		return nil
	case deletionrequest.StatusCompleted:
		uc.log.Info("anonymize skipped", "reason", "already_completed", "request_id", dr.ID)
		return nil
	}

	locale := pkglocale.EN
	if u, err := uc.userRepo.FindByID(ctx, dr.UserID); err != nil {
		return fmt.Errorf("anonymize: lookup user: %w", err)
	} else if u != nil {
		locale = u.PreferredLocale
	}

	pdfURLs, err := uc.testResultRepo.FindPDFURLsByUser(ctx, dr.UserID)
	if err != nil {
		return fmt.Errorf("anonymize: list pdf urls: %w", err)
	}
	for _, rawURL := range pdfURLs {
		if err := uc.pdfStorage.DeleteByURL(ctx, rawURL); err != nil {
			uc.log.Error("anonymize failed", "step", "delete_pdf", "request_id", dr.ID, "error", err)
			return fmt.Errorf("anonymize: delete pdf object: %w", err)
		}
	}

	scrubbedEmail := fmt.Sprintf("deleted-%s@anonymized.invalid", dr.UserID)

	txErr := uc.db.Transaction(func(tx *gorm.DB) error {
		if err := pgaccount.NewUserRepository(tx, uc.log).Anonymize(ctx, dr.UserID, scrubbedEmail); err != nil {
			return fmt.Errorf("tx: anonymize user: %w", err)
		}
		if err := pgaccount.NewGuestSessionRepository(tx, uc.log).AnonymizeClaimedByUser(ctx, dr.UserID); err != nil {
			return fmt.Errorf("tx: anonymize guest sessions: %w", err)
		}
		if err := pgassessment.NewTestResultRepository(tx, uc.log).ScrubPersonalDataByUser(ctx, dr.UserID); err != nil {
			return fmt.Errorf("tx: scrub test results: %w", err)
		}
		return nil
	})
	if txErr != nil {
		uc.log.Error("anonymize failed", "step", "scrub_transaction", "request_id", dr.ID, "error", txErr)
		return fmt.Errorf("anonymize: %w", txErr)
	}

	now := time.Now()
	if err := uc.deleteRepo.UpdateStatus(ctx, dr.ID, deletionrequest.StatusCompleted, &now); err != nil {
		return fmt.Errorf("anonymize: mark completed: %w", err)
	}

	payload := taskqueue.SendEmailPayload{
		Type:   "deletion_confirmed",
		UserID: dr.UserID,
		Email:  dr.NotificationEmail,
		Locale: locale,
	}
	if err := uc.dispatcher.EnqueueEmail(ctx, payload, taskqueue.QueueDefault); err != nil {
		uc.log.Warn("failed to enqueue deletion confirmation email", "request_id", dr.ID, "error", err)
	}

	uc.log.Info("user anonymized", "user_id", dr.UserID, "request_id", dr.ID, "pdfs_deleted", len(pdfURLs))
	return nil
}
