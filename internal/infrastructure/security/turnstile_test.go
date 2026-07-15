package security

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

func testLogger() logger.Logger { return logger.NewLogger("test") }

func newTestTurnstileClient(url string) *turnstileClient {
	return &turnstileClient{
		httpClient: http.DefaultClient,
		secretKey:  "test-secret",
		verifyURL:  url,
		log:        testLogger(),
	}
}

func TestTurnstileVerify_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	ok, err := newTestTurnstileClient(srv.URL).Verify(context.Background(), "token", "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected verification to pass on success:true")
	}
}

// An explicit Cloudflare "not successful" verdict must fail CLOSED, not open.
func TestTurnstileVerify_ExplicitFailure_FailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":false,"error-codes":["invalid-input-response"]}`))
	}))
	defer srv.Close()

	ok, err := newTestTurnstileClient(srv.URL).Verify(context.Background(), "bad-token", "")
	if err != nil {
		t.Fatalf("expected no error on an explicit verdict, got: %v", err)
	}
	if ok {
		t.Fatal("expected verification to fail closed on success:false")
	}
}

// Transport/5xx/malformed-JSON failures must fail OPEN —
// a Cloudflare outage degrades bot protection, it must not block signup/login.
func TestTurnstileVerify_ServerError_FailsOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ok, err := newTestTurnstileClient(srv.URL).Verify(context.Background(), "token", "")
	if err == nil {
		t.Fatal("expected an error to be returned alongside the fail-open true")
	}
	if !ok {
		t.Fatal("expected fail-open (true) on a 5xx from Cloudflare")
	}
}

func TestTurnstileVerify_MalformedJSON_FailsOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{not json`))
	}))
	defer srv.Close()

	ok, err := newTestTurnstileClient(srv.URL).Verify(context.Background(), "token", "")
	if err == nil {
		t.Fatal("expected a decode error")
	}
	if !ok {
		t.Fatal("expected fail-open (true) on malformed JSON")
	}
}

func TestTurnstileVerify_Timeout_FailsOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never respond within the client's context deadline below.
		<-r.Context().Done()
	}))
	defer srv.Close()

	client := newTestTurnstileClient(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 1)
	defer cancel()

	ok, err := client.Verify(ctx, "token", "")
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if !ok {
		t.Fatal("expected fail-open (true) on timeout")
	}
}

// NewTurnstileVerifier("") must yield a verifier that always passes without
// making any HTTP call at all — the local-dev bypass contract.
func TestNewTurnstileVerifier_EmptySecret_NoopAlwaysPasses(t *testing.T) {
	v := NewTurnstileVerifier("", testLogger())
	ok, err := v.Verify(context.Background(), "anything", "")
	if err != nil || !ok {
		t.Fatalf("expected noop verifier to always pass, got ok=%v err=%v", ok, err)
	}
}
