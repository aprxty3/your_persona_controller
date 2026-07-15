package assessment

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
)

// TestResultRepository implements testresult.TestResultRepository backed by PostgreSQL via GORM.
type TestResultRepository struct {
	db  *gorm.DB
	log logger.Logger
}

// NewTestResultRepository constructs a TestResultRepository.
func NewTestResultRepository(db *gorm.DB, log logger.Logger) testresult.TestResultRepository {
	return &TestResultRepository{db: db, log: log.With("repository", "testresult")}
}

// Create inserts a new test result.
func (r *TestResultRepository) Create(ctx context.Context, res *testresult.TestResult) error {
	m, err := toModel(res)
	if err != nil {
		return err
	}
	return postgres.LogQueryError(r.log, "Create", r.db.WithContext(ctx).Create(&m).Error)
}

// FindByID retrieves a test result by its UUID. Returns nil, nil if not found.
func (r *TestResultRepository) FindByID(ctx context.Context, id string) (*testresult.TestResult, error) {
	var m postgres.TestResultModel
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if postgres.IsNotFound(err) {
		return nil, nil
	}
	if err := postgres.LogQueryError(r.log, "FindByID", err); err != nil {
		return nil, err
	}
	return toEntity(&m)
}

// FindByShareToken retrieves a test result by its share token. Returns nil, nil if not found.
func (r *TestResultRepository) FindByShareToken(ctx context.Context, shareToken string) (*testresult.TestResult, error) {
	var m postgres.TestResultModel
	err := r.db.WithContext(ctx).First(&m, "share_token = ?", shareToken).Error
	if postgres.IsNotFound(err) {
		return nil, nil
	}
	if err := postgres.LogQueryError(r.log, "FindByShareToken", err); err != nil {
		return nil, err
	}
	return toEntity(&m)
}

// Update saves all mutable fields of the test result.
func (r *TestResultRepository) Update(ctx context.Context, res *testresult.TestResult) error {
	m, err := toModel(res)
	if err != nil {
		return err
	}
	return postgres.LogQueryError(r.log, "Update", r.db.WithContext(ctx).Save(&m).Error)
}

// startOfCurrentMonthJakarta returns the first instant of the current
// calendar month in Asia/Jakarta — the quota month boundary.
func startOfCurrentMonthJakarta() time.Time {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.Local
	}
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
}

// CountMonthlyUsage counts completed/fallback_static results for the given user in the current month in Asia/Jakarta timezone.
func (r *TestResultRepository) CountMonthlyUsage(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&postgres.TestResultModel{}).
		Where("user_id = ? AND status IN (?, ?) AND created_at >= ?",
			userID, string(testresult.StatusCompleted), string(testresult.StatusFallbackStatic), startOfCurrentMonthJakarta()).
		Count(&count).Error
	if err := postgres.LogQueryError(r.log, "CountMonthlyUsage", err); err != nil {
		return 0, err
	}
	return count, nil
}

// CountMonthlyUsageByGuestSession counts completed/fallback_static results for the given guest session in the current month in Asia/Jakarta timezone.
func (r *TestResultRepository) CountMonthlyUsageByGuestSession(ctx context.Context, guestSessionID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&postgres.TestResultModel{}).
		Where("guest_session_id = ? AND status IN (?, ?) AND created_at >= ?",
			guestSessionID, string(testresult.StatusCompleted), string(testresult.StatusFallbackStatic), startOfCurrentMonthJakarta()).
		Count(&count).Error
	if err := postgres.LogQueryError(r.log, "CountMonthlyUsageByGuestSession", err); err != nil {
		return 0, err
	}
	return count, nil
}

// CountCompletedByUser counts every completed/fallback_static result the user
// has ever submitted (all-time, unlike CountMonthlyUsage).
func (r *TestResultRepository) CountCompletedByUser(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&postgres.TestResultModel{}).
		Where("user_id = ? AND status IN (?, ?)", userID, string(testresult.StatusCompleted), string(testresult.StatusFallbackStatic)).
		Count(&count).Error
	if err := postgres.LogQueryError(r.log, "CountCompletedByUser", err); err != nil {
		return 0, fmt.Errorf("count completed by user: %w", err)
	}
	return count, nil
}

// FindExpiredGuestResults returns guest results that have passed their expires_at.
func (r *TestResultRepository) FindExpiredGuestResults(ctx context.Context) ([]testresult.TestResult, error) {
	var models []postgres.TestResultModel
	err := r.db.WithContext(ctx).
		Where("guest_session_id IS NOT NULL AND expires_at < ?", time.Now()).
		Find(&models).Error
	if err := postgres.LogQueryError(r.log, "FindExpiredGuestResults", err); err != nil {
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
	return postgres.LogQueryError(r.log, "UpdatePDFStatus", err)
}

// ReassignGuestResults binds completed/fallback_static results owned by guestSessionID to userID.
func (r *TestResultRepository) ReassignGuestResults(ctx context.Context, userID, guestSessionID string) error {
	err := r.db.WithContext(ctx).Model(&postgres.TestResultModel{}).
		Where("guest_session_id = ? AND status IN (?, ?)", guestSessionID, string(testresult.StatusCompleted), string(testresult.StatusFallbackStatic)).
		Updates(map[string]interface{}{
			"user_id":          userID,
			"guest_session_id": nil,
		}).Error
	if err := postgres.LogQueryError(r.log, "ReassignGuestResults", err); err != nil {
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
	if err := postgres.LogQueryError(r.log, "CountCompletedByGuestSession", err); err != nil {
		return 0, fmt.Errorf("count completed by guest session: %w", err)
	}
	return count, nil
}

// FindPDFURLsByUser returns every non-null pdf_url owned by the user.
func (r *TestResultRepository) FindPDFURLsByUser(ctx context.Context, userID string) ([]string, error) {
	var urls []string
	err := r.db.WithContext(ctx).Model(&postgres.TestResultModel{}).
		Where("user_id = ? AND pdf_url IS NOT NULL", userID).
		Pluck("pdf_url", &urls).Error
	if err := postgres.LogQueryError(r.log, "FindPDFURLsByUser", err); err != nil {
		return nil, err
	}
	return urls, nil
}

// ScrubPersonalDataByUser nulls ai_summary_text and pdf_url on all the user's results.
func (r *TestResultRepository) ScrubPersonalDataByUser(ctx context.Context, userID string) error {
	err := r.db.WithContext(ctx).Model(&postgres.TestResultModel{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{
			"ai_summary_text": nil,
			"pdf_url":         nil,
		}).Error
	return postgres.LogQueryError(r.log, "ScrubPersonalDataByUser", err)
}

// FindHistoryByUser returns a page of the user's test results ordered newest-first,
// alongside the total row count so callers can compute pagination metadata.
func (r *TestResultRepository) FindHistoryByUser(ctx context.Context, userID string, page, limit int) ([]testresult.TestResult, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	var total int64
	err := r.db.WithContext(ctx).Model(&postgres.TestResultModel{}).
		Where("user_id = ?", userID).
		Count(&total).Error
	if err := postgres.LogQueryError(r.log, "FindHistoryByUser.count", err); err != nil {
		return nil, 0, fmt.Errorf("find history by user: count: %w", err)
	}

	var models []postgres.TestResultModel
	err = r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&models).Error
	if err := postgres.LogQueryError(r.log, "FindHistoryByUser.list", err); err != nil {
		return nil, 0, fmt.Errorf("find history by user: list: %w", err)
	}

	results := make([]testresult.TestResult, len(models))
	for i, m := range models {
		res, err := toEntity(&m)
		if err != nil {
			return nil, 0, err
		}
		results[i] = *res
	}
	return results, total, nil
}

// DeleteByID hard-deletes a single test result row — used by the Guest TTL
// purge job after its R2 PDF object has already been removed.
func (r *TestResultRepository) DeleteByID(ctx context.Context, id string) error {
	err := r.db.WithContext(ctx).Delete(&postgres.TestResultModel{}, "id = ?", id).Error
	return postgres.LogQueryError(r.log, "DeleteByID", err)
}

func toModel(res *testresult.TestResult) (postgres.TestResultModel, error) {
	traits := []byte("{}")
	if res.TraitScores != nil {
		var err error
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
