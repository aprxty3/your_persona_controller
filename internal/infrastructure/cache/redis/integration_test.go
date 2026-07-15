//go:build integration

package redis

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment/dto"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	goredis "github.com/redis/go-redis/v9"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

var sharedClient *goredis.Client

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		panic("integration: start redis container: " + err.Error())
	}
	defer func() { _ = container.Terminate(ctx) }()

	connStr, err := container.ConnectionString(ctx)
	if err != nil {
		panic("integration: get connection string: " + err.Error())
	}
	addr := strings.TrimPrefix(connStr, "redis://")

	client, err := NewRedisClient(addr, "", 0)
	if err != nil {
		panic("integration: connect to test redis: " + err.Error())
	}
	sharedClient = client

	os.Exit(m.Run())
}

func testLog() logger.Logger { return logger.NewLogger("test") }

// ConsumeResetJTI uses Redis's atomic GETDEL — under concurrent consumption
// of the SAME jti, exactly one caller must get the real userID back, the
// other must see the empty "already consumed" result. This is the guarantee
// ResetPassword's single-use contract relies on.
func TestIntegration_ConsumeResetJTI_ConcurrentlyExactlyOneWinner(t *testing.T) {
	store := NewTokenStore(sharedClient)
	ctx := context.Background()
	jti := "integration-test-jti-" + time.Now().Format("150405.000000")

	if err := store.StoreResetJTI(ctx, jti, "user-1", time.Minute); err != nil {
		t.Fatalf("StoreResetJTI failed: %v", err)
	}

	const attempts = 2
	results := make([]string, attempts)
	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func(i int) {
			defer wg.Done()
			userID, err := store.ConsumeResetJTI(ctx, jti)
			if err != nil {
				t.Errorf("ConsumeResetJTI goroutine %d failed: %v", i, err)
				return
			}
			results[i] = userID
		}(i)
	}
	wg.Wait()

	wins := 0
	for _, r := range results {
		if r == "user-1" {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("expected exactly 1 winner across %d concurrent consumers, got %d (results=%v)", attempts, wins, results)
	}
}

// ReleaseLock's CAS Lua script must refuse to delete a key whose value no
// longer matches the token this instance acquired it with (e.g. because it
// expired and a different process re-acquired it in the meantime).
func TestIntegration_ReleaseLock_StaleToken_DoesNotDeleteNewOwnersKey(t *testing.T) {
	svc := NewDistributedLockService(sharedClient).(*DistributedLockService)
	ctx := context.Background()
	key := "integration-test-lock:" + time.Now().Format("150405.000000")

	acquired, err := svc.AcquireLock(ctx, key, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("expected initial acquire to succeed, got acquired=%v err=%v", acquired, err)
	}

	// Simulate the key being taken over by a different owner in between
	// (e.g. TTL expiry + re-acquire by another process) by overwriting its
	// value directly — svc's in-memory token still points at the OLD value.
	if err := sharedClient.Set(ctx, key, "someone-elses-token", time.Minute).Err(); err != nil {
		t.Fatalf("simulate new owner: %v", err)
	}

	if err := svc.ReleaseLock(ctx, key); err != nil {
		t.Fatalf("ReleaseLock returned an unexpected error: %v", err)
	}

	val, err := sharedClient.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("expected the key to still exist after a stale-token release, got error: %v", err)
	}
	if val != "someone-elses-token" {
		t.Fatalf("expected the new owner's value to be untouched, got %q", val)
	}
}

// A legitimate release (matching token) must actually delete the key.
func TestIntegration_ReleaseLock_MatchingToken_Deletes(t *testing.T) {
	svc := NewDistributedLockService(sharedClient).(*DistributedLockService)
	ctx := context.Background()
	key := "integration-test-lock-clean:" + time.Now().Format("150405.000000")

	acquired, err := svc.AcquireLock(ctx, key, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("expected acquire to succeed, got acquired=%v err=%v", acquired, err)
	}
	if err := svc.ReleaseLock(ctx, key); err != nil {
		t.Fatalf("ReleaseLock failed: %v", err)
	}

	exists, err := sharedClient.Exists(ctx, key).Result()
	if err != nil {
		t.Fatalf("exists check failed: %v", err)
	}
	if exists != 0 {
		t.Fatal("expected the key to be deleted after a matching-token release")
	}
}

// Idempotency Check/Save round-trip: a saved response is returned verbatim
// on a matching payload hash, and flagged as reused on a mismatching one.
func TestIntegration_IdempotencyService_CheckSaveRoundTrip(t *testing.T) {
	svc := NewIdempotencyService(sharedClient, testLog())
	ctx := context.Background()
	key := "idempotency_key:integration-test-" + time.Now().Format("150405.000000")

	resp := &dto.SubmitResponse{ResultID: "result-abc", MBTIType: "ENTJ", Status: "completed"}
	if err := svc.Save(ctx, key, "hash-a", resp, time.Minute); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := svc.Check(ctx, key, "hash-a")
	if err != nil {
		t.Fatalf("expected a matching-hash Check to succeed, got: %v", err)
	}
	if got == nil || got.ResultID != resp.ResultID {
		t.Fatalf("expected the saved response back, got %+v", got)
	}

	_, err = svc.Check(ctx, key, "hash-b")
	if err == nil {
		t.Fatal("expected a mismatching payload hash to be rejected")
	}
}

func TestIntegration_IdempotencyService_Check_CacheMiss(t *testing.T) {
	svc := NewIdempotencyService(sharedClient, testLog())
	ctx := context.Background()

	got, err := svc.Check(ctx, "idempotency_key:never-saved-"+time.Now().Format("150405.000000"), "any-hash")
	if err != nil {
		t.Fatalf("expected a cache miss to be a nil,nil result, got error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil on cache miss, got %+v", got)
	}
}
