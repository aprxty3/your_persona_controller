//go:build integration

package deletionrequest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/deletionrequest"
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

	if err := db.AutoMigrate(&postgres.DataDeletionRequestModel{}); err != nil {
		panic("integration: automigrate: " + err.Error())
	}

	sharedDB = db
	os.Exit(m.Run())
}

func testLog() logger.Logger { return logger.NewLogger("test") }

// FindActiveByUserID must ignore terminal-status rows even when they're the
// most recent by RequestedAt, and pick the active (pending_grace/processing)
// one instead.
func TestIntegration_FindActiveByUserID_IgnoresTerminalRows(t *testing.T) {
	repo := NewRepository(sharedDB, testLog())
	ctx := context.Background()
	userID := uuid.New().String()

	old := &postgres.DataDeletionRequestModel{
		ID: uuid.New().String(), UserID: userID, NotificationEmail: "a@example.com",
		Status: string(deletionrequest.StatusCancelled), RequestedAt: time.Now().Add(-48 * time.Hour),
	}
	active := &postgres.DataDeletionRequestModel{
		ID: uuid.New().String(), UserID: userID, NotificationEmail: "a@example.com",
		Status: string(deletionrequest.StatusPendingGrace), RequestedAt: time.Now().Add(-1 * time.Hour),
	}
	if err := sharedDB.Create(old).Error; err != nil {
		t.Fatalf("fixture setup failed: %v", err)
	}
	if err := sharedDB.Create(active).Error; err != nil {
		t.Fatalf("fixture setup failed: %v", err)
	}

	found, err := repo.FindActiveByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found == nil {
		t.Fatal("expected an active request to be found")
	}
	if found.ID != active.ID {
		t.Fatalf("expected the pending_grace request (%s) to be returned, got %s", active.ID, found.ID)
	}
}

func TestIntegration_FindActiveByUserID_NoneActive_ReturnsNilNil(t *testing.T) {
	repo := NewRepository(sharedDB, testLog())
	ctx := context.Background()
	userID := uuid.New().String()

	completed := &postgres.DataDeletionRequestModel{
		ID: uuid.New().String(), UserID: userID, NotificationEmail: "a@example.com",
		Status: string(deletionrequest.StatusCompleted), RequestedAt: time.Now(),
	}
	if err := sharedDB.Create(completed).Error; err != nil {
		t.Fatalf("fixture setup failed: %v", err)
	}

	found, err := repo.FindActiveByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != nil {
		t.Fatalf("expected no active request, got %+v", found)
	}
}

// TransitionStatus is a compare-and-swap: it must only take effect when the
// row is still in the expected `from` state, and report false (no-op)
// otherwise — this is what prevents a duplicate worker run from re-processing
// a request that already moved on.
func TestIntegration_TransitionStatus_CompareAndSwap(t *testing.T) {
	repo := NewRepository(sharedDB, testLog())
	ctx := context.Background()
	m := &postgres.DataDeletionRequestModel{
		ID: uuid.New().String(), UserID: uuid.New().String(), NotificationEmail: "a@example.com",
		Status: string(deletionrequest.StatusPendingGrace), RequestedAt: time.Now(),
	}
	if err := sharedDB.Create(m).Error; err != nil {
		t.Fatalf("fixture setup failed: %v", err)
	}

	moved, err := repo.TransitionStatus(ctx, m.ID, deletionrequest.StatusPendingGrace, deletionrequest.StatusProcessing, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !moved {
		t.Fatal("expected the first transition (pending_grace -> processing) to succeed")
	}

	// Second attempt: row is now "processing", not "pending_grace" — the CAS
	// precondition no longer holds, so this must be a no-op reporting false.
	movedAgain, err := repo.TransitionStatus(ctx, m.ID, deletionrequest.StatusPendingGrace, deletionrequest.StatusProcessing, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if movedAgain {
		t.Fatal("expected the second transition to be rejected (row is no longer pending_grace)")
	}

	var stored postgres.DataDeletionRequestModel
	if err := sharedDB.First(&stored, "id = ?", m.ID).Error; err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if stored.Status != string(deletionrequest.StatusProcessing) {
		t.Fatalf("expected status to remain %q, got %q", deletionrequest.StatusProcessing, stored.Status)
	}
}

func TestIntegration_TransitionStatus_SetsCompletedAt(t *testing.T) {
	repo := NewRepository(sharedDB, testLog())
	ctx := context.Background()
	m := &postgres.DataDeletionRequestModel{
		ID: uuid.New().String(), UserID: uuid.New().String(), NotificationEmail: "a@example.com",
		Status: string(deletionrequest.StatusProcessing), RequestedAt: time.Now(),
	}
	if err := sharedDB.Create(m).Error; err != nil {
		t.Fatalf("fixture setup failed: %v", err)
	}

	completedAt := time.Now()
	moved, err := repo.TransitionStatus(ctx, m.ID, deletionrequest.StatusProcessing, deletionrequest.StatusCompleted, &completedAt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !moved {
		t.Fatal("expected the transition to succeed")
	}

	var stored postgres.DataDeletionRequestModel
	if err := sharedDB.First(&stored, "id = ?", m.ID).Error; err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if stored.CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}
}

// FindExpiredGracePeriod must only return pending_grace rows whose grace
// period has actually elapsed (application.DeletionGracePeriod), and must
// ignore rows in other statuses regardless of age.
func TestIntegration_FindExpiredGracePeriod_OnlyOldPendingGraceIncluded(t *testing.T) {
	repo := NewRepository(sharedDB, testLog())
	ctx := context.Background()

	expired := &postgres.DataDeletionRequestModel{
		ID: uuid.New().String(), UserID: uuid.New().String(), NotificationEmail: "a@example.com",
		Status: string(deletionrequest.StatusPendingGrace), RequestedAt: time.Now().Add(-application.DeletionGracePeriod - time.Hour),
	}
	stillInGrace := &postgres.DataDeletionRequestModel{
		ID: uuid.New().String(), UserID: uuid.New().String(), NotificationEmail: "a@example.com",
		Status: string(deletionrequest.StatusPendingGrace), RequestedAt: time.Now().Add(-application.DeletionGracePeriod + time.Hour),
	}
	oldButProcessing := &postgres.DataDeletionRequestModel{
		ID: uuid.New().String(), UserID: uuid.New().String(), NotificationEmail: "a@example.com",
		Status: string(deletionrequest.StatusProcessing), RequestedAt: time.Now().Add(-application.DeletionGracePeriod - time.Hour),
	}
	for _, m := range []*postgres.DataDeletionRequestModel{expired, stillInGrace, oldButProcessing} {
		if err := sharedDB.Create(m).Error; err != nil {
			t.Fatalf("fixture setup failed: %v", err)
		}
	}

	results, err := repo.FindExpiredGracePeriod(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := make(map[string]bool, len(results))
	for _, r := range results {
		found[r.ID] = true
	}
	if !found[expired.ID] {
		t.Error("expected the expired pending_grace request to be included")
	}
	if found[stillInGrace.ID] {
		t.Error("expected the still-in-grace-period request to be excluded")
	}
	if found[oldButProcessing.ID] {
		t.Error("expected the old but non-pending_grace request to be excluded")
	}
}
