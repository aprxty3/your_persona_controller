//go:build integration

package account

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/google/uuid"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"gorm.io/gorm"
)

// sharedDB is a single Postgres testcontainer reused across every test in
// this file — see internal/infrastructure/persistence/postgres/assessment/integration_test.go
// for the rationale (one container per run, not per test).
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

	// Retry: testcontainers reports the container ready after Postgres's FIRST
	// startup (post-initdb), but Postgres immediately restarts once more before
	// actually serving — a connection landing in that restart window gets
	// "unexpected EOF"/"connection reset by peer", not a clean refused-connection.
	var db *gorm.DB
	for attempt := 1; attempt <= 5; attempt++ {
		db, err = postgres.NewPostgresDB(connStr)
		if err == nil {
			break
		}
		if attempt == 5 {
			panic("integration: connect to test postgres: " + err.Error())
		}
		time.Sleep(time.Duration(attempt) * time.Second)
	}

	if err := db.AutoMigrate(
		&postgres.UserModel{},
		&postgres.GuestSessionModel{},
		&postgres.TestResultModel{},
		&postgres.ReferralCodeModel{},
		&postgres.ReferralEventModel{},
		&postgres.VerificationTokenModel{},
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

func newTestUser() *postgres.UserModel {
	return &postgres.UserModel{
		ID:           uuid.New().String(),
		Email:        uuid.New().String() + "@example.com",
		PasswordHash: "hashed",
	}
}

// --- UserRepository ---

func TestIntegration_UserFindByID_NotFound_ReturnsNilNil(t *testing.T) {
	repo := NewUserRepository(sharedDB, testLog())

	u, err := repo.FindByID(context.Background(), uuid.New().String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != nil {
		t.Fatalf("expected nil user for a non-existent ID, got %+v", u)
	}
}

func TestIntegration_UserFindByEmail_NotFound_ReturnsNilNil(t *testing.T) {
	repo := NewUserRepository(sharedDB, testLog())

	u, err := repo.FindByEmail(context.Background(), "does-not-exist@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != nil {
		t.Fatalf("expected nil user for a non-existent email, got %+v", u)
	}
}

func TestIntegration_UserIncrementTokenVersion_AtomicIncrement(t *testing.T) {
	repo := NewUserRepository(sharedDB, testLog())
	ctx := context.Background()
	m := newTestUser()
	mustCreate(t, m)

	if err := repo.IncrementTokenVersion(ctx, m.ID); err != nil {
		t.Fatalf("first increment failed: %v", err)
	}
	if err := repo.IncrementTokenVersion(ctx, m.ID); err != nil {
		t.Fatalf("second increment failed: %v", err)
	}

	u, err := repo.FindByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if u.TokenVersion != 2 {
		t.Fatalf("expected token_version 2 after two increments, got %d", u.TokenVersion)
	}
}

func TestIntegration_UserUpdateLoginAttempt_SetsCounterAndLockout(t *testing.T) {
	repo := NewUserRepository(sharedDB, testLog())
	ctx := context.Background()
	m := newTestUser()
	mustCreate(t, m)

	lockedUntil := time.Now().Add(15 * time.Minute)
	if err := repo.UpdateLoginAttempt(ctx, m.ID, 5, &lockedUntil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	u, err := repo.FindByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if u.FailedLoginCount != 5 {
		t.Fatalf("expected failed_login_count 5, got %d", u.FailedLoginCount)
	}
	if u.LockedUntil == nil {
		t.Fatal("expected locked_until to be set")
	}
}

// Anonymize scrubs PII AND soft-deletes the row in a single UPDATE. FindByID
// (which does not use Unscoped) must no longer see the row afterward — that's
// the whole point of anonymization making the account permanently unusable.
func TestIntegration_UserAnonymize_ScrubsPIIAndSoftDeletes(t *testing.T) {
	repo := NewUserRepository(sharedDB, testLog())
	ctx := context.Background()
	m := newTestUser()
	m.DisplayName = "Real Name"
	m.Age = 25
	mustCreate(t, m)

	if err := repo.Anonymize(ctx, m.ID, "anonymized-"+m.ID+"@deleted.local"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	u, err := repo.FindByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != nil {
		t.Fatalf("expected FindByID to no longer see the anonymized (soft-deleted) row, got %+v", u)
	}

	var raw postgres.UserModel
	if err := sharedDB.Unscoped().First(&raw, "id = ?", m.ID).Error; err != nil {
		t.Fatalf("unscoped fetch failed: %v", err)
	}
	if raw.DisplayName != "" {
		t.Errorf("expected display_name scrubbed, got %q", raw.DisplayName)
	}
	if raw.PasswordHash != "" {
		t.Errorf("expected password_hash scrubbed, got %q", raw.PasswordHash)
	}
	if !raw.DeletedAt.Valid {
		t.Error("expected deleted_at to be set")
	}
	if raw.AnonymizedAt == nil {
		t.Error("expected anonymized_at to be set")
	}
	if raw.TokenVersion != 1 {
		t.Errorf("expected token_version incremented to 1 (invalidates existing sessions), got %d", raw.TokenVersion)
	}
}

// --- GuestSessionRepository ---

func TestIntegration_FindExpiredUnclaimed_OnlyTrueOrphansIncluded(t *testing.T) {
	repo := NewGuestSessionRepository(sharedDB, testLog())
	ctx := context.Background()
	claimedByUser := uuid.New().String()
	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	orphan := &postgres.GuestSessionModel{SessionID: uuid.New().String(), IPHash: "h", DisplayName: "d", Age: 20, Status: "s", ExpiresAt: past}
	hasResult := &postgres.GuestSessionModel{SessionID: uuid.New().String(), IPHash: "h", DisplayName: "d", Age: 20, Status: "s", ExpiresAt: past}
	claimed := &postgres.GuestSessionModel{SessionID: uuid.New().String(), IPHash: "h", DisplayName: "d", Age: 20, Status: "s", ExpiresAt: past, ClaimedByUserID: &claimedByUser}
	notExpired := &postgres.GuestSessionModel{SessionID: uuid.New().String(), IPHash: "h", DisplayName: "d", Age: 20, Status: "s", ExpiresAt: future}
	mustCreate(t, orphan)
	mustCreate(t, hasResult)
	mustCreate(t, claimed)
	mustCreate(t, notExpired)

	mustCreate(t, &postgres.TestResultModel{
		ID: uuid.New().String(), GuestSessionID: &hasResult.SessionID, ShareToken: uuid.New().String(),
		Locale: "en", Status: "completed", TraitScores: "{}",
	})

	results, err := repo.FindExpiredUnclaimed(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := make(map[string]bool, len(results))
	for _, r := range results {
		found[r.SessionID] = true
	}
	if !found[orphan.SessionID] {
		t.Error("expected the true orphan (expired, unclaimed, no result) to be included")
	}
	if found[hasResult.SessionID] {
		t.Error("expected the session with a test result to be excluded (belongs to result-driven purge)")
	}
	if found[claimed.SessionID] {
		t.Error("expected the claimed session to be excluded")
	}
	if found[notExpired.SessionID] {
		t.Error("expected the not-yet-expired session to be excluded")
	}
}

func TestIntegration_AnonymizeClaimedByUser_ScrubsOnlyMatchingRows(t *testing.T) {
	repo := NewGuestSessionRepository(sharedDB, testLog())
	ctx := context.Background()
	targetUser := uuid.New().String()
	otherUser := uuid.New().String()

	target := &postgres.GuestSessionModel{SessionID: uuid.New().String(), IPHash: "h", DisplayName: "Real Name", Age: 30, Status: "s", ExpiresAt: time.Now().Add(time.Hour), ClaimedByUserID: &targetUser}
	other := &postgres.GuestSessionModel{SessionID: uuid.New().String(), IPHash: "h", DisplayName: "Other Name", Age: 31, Status: "s", ExpiresAt: time.Now().Add(time.Hour), ClaimedByUserID: &otherUser}
	mustCreate(t, target)
	mustCreate(t, other)

	if err := repo.AnonymizeClaimedByUser(ctx, targetUser); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	afterTarget, err := repo.FindBySessionID(ctx, target.SessionID)
	if err != nil {
		t.Fatalf("fetch target failed: %v", err)
	}
	afterOther, err := repo.FindBySessionID(ctx, other.SessionID)
	if err != nil {
		t.Fatalf("fetch other failed: %v", err)
	}
	if afterTarget.DisplayName != "" {
		t.Errorf("expected target session display_name scrubbed, got %q", afterTarget.DisplayName)
	}
	if afterOther.DisplayName != "Other Name" {
		t.Errorf("expected other user's session to survive untouched, got %q", afterOther.DisplayName)
	}
}

func TestIntegration_DeleteBySessionID_RemovesRow(t *testing.T) {
	repo := NewGuestSessionRepository(sharedDB, testLog())
	ctx := context.Background()
	m := &postgres.GuestSessionModel{SessionID: uuid.New().String(), IPHash: "h", DisplayName: "d", Age: 20, Status: "s", ExpiresAt: time.Now().Add(time.Hour)}
	mustCreate(t, m)

	if err := repo.DeleteBySessionID(ctx, m.SessionID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found, err := repo.FindBySessionID(ctx, m.SessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != nil {
		t.Fatalf("expected session to be gone after delete, got %+v", found)
	}
}

// --- ReferralRepository ---

func TestIntegration_ReferralCode_CreateAndFind_RoundTrip(t *testing.T) {
	repo := NewReferralRepository(sharedDB, testLog())
	ctx := context.Background()
	userID := uuid.New().String()
	code := &account.ReferralCode{ID: uuid.New().String(), UserID: userID, Code: "REF" + uuid.New().String()[:8], CreatedAt: time.Now()}

	if err := repo.CreateCode(ctx, code); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byUser, err := repo.FindCodeByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if byUser == nil || byUser.Code != code.Code {
		t.Fatalf("expected to find the code by user id, got %+v", byUser)
	}

	byCode, err := repo.FindCodeByCode(ctx, code.Code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if byCode == nil || byCode.UserID != userID {
		t.Fatalf("expected to find the code by code string, got %+v", byCode)
	}
}

func TestIntegration_CountEventsByCodeID_FiltersByEventType(t *testing.T) {
	repo := NewReferralRepository(sharedDB, testLog())
	ctx := context.Background()
	codeID := uuid.New().String()

	events := []*account.ReferralEvent{
		{ID: uuid.New().String(), ReferralCodeID: codeID, ReferredUserID: uuid.New().String(), EventType: account.EventTypeSignup, CreatedAt: time.Now()},
		{ID: uuid.New().String(), ReferralCodeID: codeID, ReferredUserID: uuid.New().String(), EventType: account.EventTypeSignup, CreatedAt: time.Now()},
		{ID: uuid.New().String(), ReferralCodeID: codeID, ReferredUserID: uuid.New().String(), EventType: account.EventTypeTestCompleted, CreatedAt: time.Now()},
	}
	for _, e := range events {
		if err := repo.CreateEvent(ctx, e); err != nil {
			t.Fatalf("fixture setup failed: %v", err)
		}
	}

	signups, err := repo.CountEventsByCodeID(ctx, codeID, account.EventTypeSignup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if signups != 2 {
		t.Errorf("expected 2 signup events, got %d", signups)
	}

	completions, err := repo.CountEventsByCodeID(ctx, codeID, account.EventTypeTestCompleted)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if completions != 1 {
		t.Errorf("expected 1 test_completed event, got %d", completions)
	}
}

// --- VerificationTokenRepository ---

// Multiple active tokens can exist transiently (e.g. resend-OTP without
// expiring the old one first) — FindActiveByUserAndType must deterministically
// pick the most recently created one (ORDER BY created_at DESC), not an
// arbitrary row.
func TestIntegration_FindActiveByUserAndType_PicksMostRecent(t *testing.T) {
	repo := NewVerificationTokenRepository(sharedDB, testLog())
	ctx := context.Background()
	userID := uuid.New().String()
	now := time.Now()

	older := &postgres.VerificationTokenModel{ID: uuid.New().String(), UserID: userID, Token: "111111", Type: string(account.TokenTypeEmailVerification), ExpiresAt: now.Add(time.Hour)}
	mustCreate(t, older)
	sharedDB.Model(older).Update("created_at", now.Add(-time.Minute))

	newer := &postgres.VerificationTokenModel{ID: uuid.New().String(), UserID: userID, Token: "222222", Type: string(account.TokenTypeEmailVerification), ExpiresAt: now.Add(time.Hour)}
	mustCreate(t, newer)

	active, err := repo.FindActiveByUserAndType(ctx, userID, account.TokenTypeEmailVerification)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active == nil {
		t.Fatal("expected an active token")
	}
	if active.ID != newer.ID {
		t.Fatalf("expected the most recently created token (%s) to be picked, got %s", newer.ID, active.ID)
	}
}

func TestIntegration_FindActiveByUserAndType_ExcludesExpiredAndUsed(t *testing.T) {
	repo := NewVerificationTokenRepository(sharedDB, testLog())
	ctx := context.Background()
	userID := uuid.New().String()
	now := time.Now()
	usedAt := now

	expired := &postgres.VerificationTokenModel{ID: uuid.New().String(), UserID: userID, Token: "1", Type: string(account.TokenTypePasswordReset), ExpiresAt: now.Add(-time.Minute)}
	used := &postgres.VerificationTokenModel{ID: uuid.New().String(), UserID: userID, Token: "2", Type: string(account.TokenTypePasswordReset), ExpiresAt: now.Add(time.Hour), UsedAt: &usedAt}
	mustCreate(t, expired)
	mustCreate(t, used)

	active, err := repo.FindActiveByUserAndType(ctx, userID, account.TokenTypePasswordReset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active != nil {
		t.Fatalf("expected no active token (all expired/used), got %+v", active)
	}
}

func TestIntegration_ExpireAllActiveForUser_ForceExpiresOnlyMatchingTypeAndUser(t *testing.T) {
	repo := NewVerificationTokenRepository(sharedDB, testLog())
	ctx := context.Background()
	userID := uuid.New().String()
	otherUserID := uuid.New().String()
	future := time.Now().Add(time.Hour)

	target := &postgres.VerificationTokenModel{ID: uuid.New().String(), UserID: userID, Token: "1", Type: string(account.TokenTypeEmailVerification), ExpiresAt: future}
	wrongType := &postgres.VerificationTokenModel{ID: uuid.New().String(), UserID: userID, Token: "2", Type: string(account.TokenTypePasswordReset), ExpiresAt: future}
	otherUser := &postgres.VerificationTokenModel{ID: uuid.New().String(), UserID: otherUserID, Token: "3", Type: string(account.TokenTypeEmailVerification), ExpiresAt: future}
	mustCreate(t, target)
	mustCreate(t, wrongType)
	mustCreate(t, otherUser)

	if err := repo.ExpireAllActiveForUser(ctx, userID, account.TokenTypeEmailVerification); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	activeTarget, err := repo.FindActiveByUserAndType(ctx, userID, account.TokenTypeEmailVerification)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if activeTarget != nil {
		t.Errorf("expected the target user's email_verification token to be expired, still active: %+v", activeTarget)
	}

	activeWrongType, err := repo.FindActiveByUserAndType(ctx, userID, account.TokenTypePasswordReset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if activeWrongType == nil {
		t.Error("expected the same user's password_reset token to survive (different type)")
	}

	activeOtherUser, err := repo.FindActiveByUserAndType(ctx, otherUserID, account.TokenTypeEmailVerification)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if activeOtherUser == nil {
		t.Error("expected the other user's token to survive untouched")
	}
}

func TestIntegration_IncrementAttemptCount_AtomicIncrement(t *testing.T) {
	repo := NewVerificationTokenRepository(sharedDB, testLog())
	ctx := context.Background()
	m := &postgres.VerificationTokenModel{ID: uuid.New().String(), UserID: uuid.New().String(), Token: "1", Type: string(account.TokenTypeEmailVerification), ExpiresAt: time.Now().Add(time.Hour)}
	mustCreate(t, m)

	if err := repo.IncrementAttemptCount(ctx, m.ID); err != nil {
		t.Fatalf("first increment failed: %v", err)
	}
	if err := repo.IncrementAttemptCount(ctx, m.ID); err != nil {
		t.Fatalf("second increment failed: %v", err)
	}

	var stored postgres.VerificationTokenModel
	if err := sharedDB.First(&stored, "id = ?", m.ID).Error; err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if stored.AttemptCount != 2 {
		t.Fatalf("expected attempt_count 2 after two increments, got %d", stored.AttemptCount)
	}
}
