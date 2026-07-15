//go:build integration

package assessment

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/content"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/google/uuid"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"gorm.io/gorm"
)

// sharedDB is a single Postgres testcontainer reused across every test in
// this file — spinning one container per test would dominate the run time
// for no isolation benefit, since every test seeds its own uuid-scoped rows.
var sharedDB *gorm.DB

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("your_persona_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
	)
	if err != nil {
		panic("integration: start postgres container: " + err.Error())
	}
	defer func() { _ = container.Terminate(ctx) }()

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic("integration: get connection string: " + err.Error())
	}

	db, err := postgres.NewPostgresDB(connStr)
	if err != nil {
		panic("integration: connect to test postgres: " + err.Error())
	}

	// Same model list/order as cmd/migrate/main.go — kept manually in sync
	// since this is a throwaway test schema, not the real migration path.
	if err := db.AutoMigrate(
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
	); err != nil {
		panic("integration: automigrate: " + err.Error())
	}

	sharedDB = db
	os.Exit(m.Run())
}

func testLog() logger.Logger { return logger.NewLogger("test") }

func mustCreate(t *testing.T, v interface{}) {
	t.Helper()
	if err := sharedDB.Create(v).Error; err != nil {
		t.Fatalf("fixture setup failed: %v", err)
	}
}

// Submitting the same (test_result_id, question_id) pair twice — the
// double-submit / retry scenario the ON CONFLICT clause exists for — must
// upsert in place, never create a duplicate row.
func TestIntegration_UpsertAnswers_OnConflict_SingleRow(t *testing.T) {
	repo := NewAnswerRepository(sharedDB, testLog())
	ctx := context.Background()
	resultID := uuid.New().String()
	questionID := uuid.New().String()

	err := repo.UpsertAnswers(ctx, resultID, []testresult.Answer{
		{ID: uuid.New().String(), TestResultID: resultID, QuestionID: questionID, Value: "3"},
	})
	if err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}

	err = repo.UpsertAnswers(ctx, resultID, []testresult.Answer{
		{ID: uuid.New().String(), TestResultID: resultID, QuestionID: questionID, Value: "5"},
	})
	if err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	var count int64
	if err := sharedDB.Model(&postgres.AnswerModel{}).
		Where("test_result_id = ? AND question_id = ?", resultID, questionID).
		Count(&count).Error; err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 row after re-submitting the same question, got %d", count)
	}

	var stored postgres.AnswerModel
	if err := sharedDB.Where("test_result_id = ? AND question_id = ?", resultID, questionID).First(&stored).Error; err != nil {
		t.Fatalf("fetch upserted row failed: %v", err)
	}
	if stored.Value != "5" {
		t.Fatalf("expected the second submission's value to win, got %q", stored.Value)
	}
}

// CountMonthlyUsage buckets by Asia/Jakarta calendar month, not UTC — a row
// exactly at the Jakarta month boundary (which sits at 17:00 UTC on the
// LAST day of the previous month, since Jakarta is UTC+7 with no DST) must
// be counted; one second earlier must not.
func TestIntegration_CountMonthlyUsage_JakartaMonthBoundary(t *testing.T) {
	repo := NewTestResultRepository(sharedDB, testLog())
	ctx := context.Background()
	userID := uuid.New().String()

	jkt, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("load Asia/Jakarta location: %v", err)
	}
	now := time.Now().In(jkt)
	boundary := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, jkt)

	included := &postgres.TestResultModel{
		ID: uuid.New().String(), UserID: &userID, ShareToken: uuid.New().String(),
		Locale: "en", Status: string(testresult.StatusCompleted), CreatedAt: boundary,
	}
	excluded := &postgres.TestResultModel{
		ID: uuid.New().String(), UserID: &userID, ShareToken: uuid.New().String(),
		Locale: "en", Status: string(testresult.StatusCompleted), CreatedAt: boundary.Add(-1 * time.Second),
	}
	mustCreate(t, included)
	mustCreate(t, excluded)
	// CreatedAt has gorm:"autoCreateTime", which overwrites an explicitly
	// set value on Create — force the exact boundary timestamps via a
	// direct UPDATE so the test controls them precisely.
	if err := sharedDB.Model(&postgres.TestResultModel{}).Where("id = ?", included.ID).Update("created_at", boundary).Error; err != nil {
		t.Fatalf("force included created_at: %v", err)
	}
	if err := sharedDB.Model(&postgres.TestResultModel{}).Where("id = ?", excluded.ID).Update("created_at", boundary.Add(-1*time.Second)).Error; err != nil {
		t.Fatalf("force excluded created_at: %v", err)
	}

	count, err := repo.CountMonthlyUsage(ctx, userID)
	if err != nil {
		t.Fatalf("CountMonthlyUsage failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 result counted (boundary row included, 1s-earlier row excluded), got %d", count)
	}
}

// ScrubEssayAnswersByUser must blank ONLY essay-type answers; Likert answers
// for the same user/result must survive untouched (they carry no PII).
func TestIntegration_ScrubEssayAnswersByUser_OnlyEssayBlanked(t *testing.T) {
	repo := NewAnswerRepository(sharedDB, testLog())
	ctx := context.Background()
	userID := uuid.New().String()

	essayQuestion := &postgres.QuestionModel{ID: uuid.New().String(), Section: "b", Type: string(content.TypeEssayPrompt), DisplayOrder: 1}
	likertQuestion := &postgres.QuestionModel{ID: uuid.New().String(), Section: "b", Type: string(content.TypeLikert), DisplayOrder: 2}
	mustCreate(t, essayQuestion)
	mustCreate(t, likertQuestion)

	result := &postgres.TestResultModel{
		ID: uuid.New().String(), UserID: &userID, ShareToken: uuid.New().String(),
		Locale: "en", Status: string(testresult.StatusCompleted),
	}
	mustCreate(t, result)

	essayAnswer := &postgres.AnswerModel{ID: uuid.New().String(), TestResultID: result.ID, QuestionID: essayQuestion.ID, Value: "my personal essay"}
	likertAnswer := &postgres.AnswerModel{ID: uuid.New().String(), TestResultID: result.ID, QuestionID: likertQuestion.ID, Value: "4"}
	mustCreate(t, essayAnswer)
	mustCreate(t, likertAnswer)

	if err := repo.ScrubEssayAnswersByUser(ctx, userID); err != nil {
		t.Fatalf("ScrubEssayAnswersByUser failed: %v", err)
	}

	var afterEssay, afterLikert postgres.AnswerModel
	if err := sharedDB.First(&afterEssay, "id = ?", essayAnswer.ID).Error; err != nil {
		t.Fatalf("fetch essay answer: %v", err)
	}
	if err := sharedDB.First(&afterLikert, "id = ?", likertAnswer.ID).Error; err != nil {
		t.Fatalf("fetch likert answer: %v", err)
	}
	if afterEssay.Value != "" {
		t.Fatalf("expected essay answer to be blanked, got %q", afterEssay.Value)
	}
	if afterLikert.Value != "4" {
		t.Fatalf("expected likert answer to survive untouched, got %q", afterLikert.Value)
	}
}

// PromptAuditLog has no user_id column of its own — DeleteByUserID resolves
// ownership via a subquery on test_results.user_id. Verify it deletes only
// the target user's logs and leaves another user's logs alone.
func TestIntegration_PromptAuditLogDeleteByUserID_SubqueryScoped(t *testing.T) {
	repo := NewPromptAuditLogRepository(sharedDB, testLog())
	ctx := context.Background()
	targetUser := uuid.New().String()
	otherUser := uuid.New().String()

	targetResult := &postgres.TestResultModel{ID: uuid.New().String(), UserID: &targetUser, ShareToken: uuid.New().String(), Locale: "en", Status: string(testresult.StatusCompleted)}
	otherResult := &postgres.TestResultModel{ID: uuid.New().String(), UserID: &otherUser, ShareToken: uuid.New().String(), Locale: "en", Status: string(testresult.StatusCompleted)}
	mustCreate(t, targetResult)
	mustCreate(t, otherResult)

	targetLog := &postgres.PromptAuditLogModel{ID: uuid.New().String(), TestResultID: targetResult.ID, RawPrompt: "p", RawResponse: "r", ExpiresAt: time.Now().Add(30 * 24 * time.Hour)}
	otherLog := &postgres.PromptAuditLogModel{ID: uuid.New().String(), TestResultID: otherResult.ID, RawPrompt: "p", RawResponse: "r", ExpiresAt: time.Now().Add(30 * 24 * time.Hour)}
	mustCreate(t, targetLog)
	mustCreate(t, otherLog)

	if err := repo.DeleteByUserID(ctx, targetUser); err != nil {
		t.Fatalf("DeleteByUserID failed: %v", err)
	}

	var targetCount, otherCount int64
	sharedDB.Model(&postgres.PromptAuditLogModel{}).Where("id = ?", targetLog.ID).Count(&targetCount)
	sharedDB.Model(&postgres.PromptAuditLogModel{}).Where("id = ?", otherLog.ID).Count(&otherCount)

	if targetCount != 0 {
		t.Fatalf("expected the target user's audit log to be deleted, found %d", targetCount)
	}
	if otherCount != 1 {
		t.Fatalf("expected the other user's audit log to survive, found %d", otherCount)
	}
}

// uniq_active_deletion_per_user is a PARTIAL unique index (only enforced
// while status is pending_grace/processing) — a second active request for
// the same user must be rejected, but a second row is fine once the first
// is cancelled/completed.
func TestIntegration_UniqueActiveDeletionPerUser_PartialIndex(t *testing.T) {
	userID := uuid.New().String()

	first := &postgres.DataDeletionRequestModel{
		ID: uuid.New().String(), UserID: userID, NotificationEmail: "a@example.com",
		Status: "pending_grace", RequestedAt: time.Now(),
	}
	if err := sharedDB.Create(first).Error; err != nil {
		t.Fatalf("expected first active deletion request to succeed: %v", err)
	}

	second := &postgres.DataDeletionRequestModel{
		ID: uuid.New().String(), UserID: userID, NotificationEmail: "a@example.com",
		Status: "pending_grace", RequestedAt: time.Now(),
	}
	err := sharedDB.Create(second).Error
	if err == nil {
		t.Fatal("expected a duplicate active deletion request to violate the partial unique index")
	}
	if !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("expected gorm.ErrDuplicatedKey (TranslateError:true), got %v", err)
	}

	// A cancelled row for the same user falls outside the partial index's
	// predicate, so it must NOT collide with the still-active first request.
	cancelled := &postgres.DataDeletionRequestModel{
		ID: uuid.New().String(), UserID: userID, NotificationEmail: "a@example.com",
		Status: "cancelled", RequestedAt: time.Now(),
	}
	if err := sharedDB.Create(cancelled).Error; err != nil {
		t.Fatalf("expected a cancelled-status row to bypass the partial index, got error: %v", err)
	}
}
