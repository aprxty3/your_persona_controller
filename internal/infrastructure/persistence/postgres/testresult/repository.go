package testresult

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
)

// TestResultRepository implements testresult.Repository backed by PostgreSQL via GORM.
type TestResultRepository struct {
	db  *gorm.DB
	log logger.Logger
}

// NewTestResultRepository constructs a TestResultRepository.
func NewTestResultRepository(db *gorm.DB, log logger.Logger) testresult.Repository {
	return &TestResultRepository{db: db, log: log.With("repository", "testresult")}
}

// Create inserts a new test result.
func (r *TestResultRepository) Create(ctx context.Context, res *testresult.TestResult) error {
	m, err := toModel(res)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		r.log.Error("query failed", "op", "Create", "error", err)
		return err
	}
	return nil
}

// FindByID retrieves a test result by its UUID. Returns nil, nil if not found.
func (r *TestResultRepository) FindByID(ctx context.Context, id string) (*testresult.TestResult, error) {
	var m postgres.TestResultModel
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		r.log.Error("query failed", "op", "FindByID", "error", err)
		return nil, err
	}
	res, err := toEntity(&m)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// FindByShareToken retrieves a test result by its share token. Returns nil, nil if not found.
func (r *TestResultRepository) FindByShareToken(ctx context.Context, shareToken string) (*testresult.TestResult, error) {
	var m postgres.TestResultModel
	err := r.db.WithContext(ctx).First(&m, "share_token = ?", shareToken).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		r.log.Error("query failed", "op", "FindByShareToken", "error", err)
		return nil, err
	}
	res, err := toEntity(&m)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// Update saves all mutable fields of the test result.
func (r *TestResultRepository) Update(ctx context.Context, res *testresult.TestResult) error {
	m, err := toModel(res)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Save(&m).Error; err != nil {
		r.log.Error("query failed", "op", "Update", "error", err)
		return err
	}
	return nil
}

// CountMonthlyUsage counts completed/fallback_static results for the given user in the current month in Asia/Jakarta timezone.
func (r *TestResultRepository) CountMonthlyUsage(ctx context.Context, userID string) (int64, error) {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.Local
	}
	now := time.Now().In(loc)
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)

	var count int64
	err = r.db.WithContext(ctx).Model(&postgres.TestResultModel{}).
		Where("(user_id = ? OR guest_session_id = ?) AND status IN (?, ?) AND created_at >= ?",
			userID, userID, string(testresult.StatusCompleted), string(testresult.StatusFallbackStatic), startOfMonth).
		Count(&count).Error
	if err != nil {
		r.log.Error("query failed", "op", "CountMonthlyUsage", "error", err)
		return 0, err
	}
	return count, nil
}

// FindExpiredGuestResults returns guest results that have passed their expires_at.
func (r *TestResultRepository) FindExpiredGuestResults(ctx context.Context) ([]testresult.TestResult, error) {
	var models []postgres.TestResultModel
	err := r.db.WithContext(ctx).
		Where("guest_session_id IS NOT NULL AND expires_at < ?", time.Now()).
		Find(&models).Error
	if err != nil {
		r.log.Error("query failed", "op", "FindExpiredGuestResults", "error", err)
		return nil, err
	}

	results := make([]testresult.TestResult, len(models))
	for i, m := range models {
		res, err := toEntity(&m)
		if err != nil {
			return nil, err
		}
		results[i] = *res
	}
	return results, nil
}

// UpdatePDFStatus updates pdf_url and pdf_status.
func (r *TestResultRepository) UpdatePDFStatus(ctx context.Context, id string, pdfURL *string, status testresult.PDFStatus) error {
	err := r.db.WithContext(ctx).Model(&postgres.TestResultModel{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"pdf_url":    pdfURL,
			"pdf_status": string(status),
		}).Error
	if err != nil {
		r.log.Error("query failed", "op", "UpdatePDFStatus", "error", err)
	}
	return err
}

// ReassignGuestResults binds completed/fallback_static results owned by guestSessionID to userID.
func (r *TestResultRepository) ReassignGuestResults(ctx context.Context, userID, guestSessionID string) error {
	err := r.db.WithContext(ctx).Model(&postgres.TestResultModel{}).
		Where("guest_session_id = ? AND status IN (?, ?)", guestSessionID, string(testresult.StatusCompleted), string(testresult.StatusFallbackStatic)).
		Updates(map[string]interface{}{
			"user_id":          userID,
			"guest_session_id": nil,
		}).Error
	if err != nil {
		r.log.Error("query failed", "op", "ReassignGuestResults", "error", err)
		return fmt.Errorf("reassign guest results: %w", err)
	}
	return nil
}

// CountCompletedByGuestSession counts test results for a guest session with completed or fallback_static status.
func (r *TestResultRepository) CountCompletedByGuestSession(ctx context.Context, guestSessionID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&postgres.TestResultModel{}).
		Where("guest_session_id = ? AND status IN (?, ?)", guestSessionID, string(testresult.StatusCompleted), string(testresult.StatusFallbackStatic)).
		Count(&count).Error
	if err != nil {
		r.log.Error("query failed", "op", "CountCompletedByGuestSession", "error", err)
		return 0, fmt.Errorf("count completed by guest session: %w", err)
	}
	return count, nil
}

func toModel(res *testresult.TestResult) (postgres.TestResultModel, error) {
	var traits []byte
	var err error
	if res.TraitScores != nil {
		traits, err = json.Marshal(res.TraitScores)
		if err != nil {
			return postgres.TestResultModel{}, fmt.Errorf("marshal trait scores: %w", err)
		}
	}
	return postgres.TestResultModel{
		ID:               res.ID,
		UserID:           res.UserID,
		GuestSessionID:   res.GuestSessionID,
		ShareToken:       res.ShareToken,
		Locale:           res.Locale,
		MascotStyle:      res.MascotStyle,
		MBTIType:         res.MBTIType,
		GritScore:        res.GritScore,
		TraitScores:      string(traits),
		AISummaryText:    res.AISummaryText,
		Status:           string(res.Status),
		WellbeingFlag:    res.WellbeingFlag,
		PDFUrl:           res.PDFUrl,
		PDFStatus:        string(res.PDFStatus),
		PromptTokens:     res.PromptTokens,
		CompletionTokens: res.CompletionTokens,
		TotalTokens:      res.TotalTokens,
		CreatedAt:        res.CreatedAt,
		ExpiresAt:        res.ExpiresAt,
	}, nil
}

func toEntity(m *postgres.TestResultModel) (*testresult.TestResult, error) {
	var traits map[string]interface{}
	if m.TraitScores != "" {
		if err := json.Unmarshal([]byte(m.TraitScores), &traits); err != nil {
			return nil, fmt.Errorf("unmarshal trait scores: %w", err)
		}
	}
	return &testresult.TestResult{
		ID:               m.ID,
		UserID:           m.UserID,
		GuestSessionID:   m.GuestSessionID,
		ShareToken:       m.ShareToken,
		Locale:           m.Locale,
		MascotStyle:      m.MascotStyle,
		MBTIType:         m.MBTIType,
		GritScore:        m.GritScore,
		TraitScores:      traits,
		AISummaryText:    m.AISummaryText,
		Status:           testresult.ResultStatus(m.Status),
		WellbeingFlag:    m.WellbeingFlag,
		PDFUrl:           m.PDFUrl,
		PDFStatus:        testresult.PDFStatus(m.PDFStatus),
		PromptTokens:     m.PromptTokens,
		CompletionTokens: m.CompletionTokens,
		TotalTokens:      m.TotalTokens,
		CreatedAt:        m.CreatedAt,
		ExpiresAt:        m.ExpiresAt,
	}, nil
}
