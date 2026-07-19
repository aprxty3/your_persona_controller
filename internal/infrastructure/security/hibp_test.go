package security

import (
	"context"
	"crypto/sha1" //nolint:gosec // mirrors the HIBP range API's k-anonymity protocol under test, not a real credential hash
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestHIBPChecker(url string) *HIBPBreachChecker {
	return &HIBPBreachChecker{
		httpClient: http.DefaultClient,
		rangeURL:   url,
		log:        testLogger(),
	}
}

func hashSuffix(password string) (prefix, suffix string) {
	sum := sha1.Sum([]byte(password)) //nolint:gosec // test fixture mirrors the HIBP API's own SHA-1 requirement
	hash := strings.ToUpper(hex.EncodeToString(sum[:]))
	return hash[:5], hash[5:]
}

func TestHIBPIsBreached_SuffixFound_ReturnsTrue(t *testing.T) {
	_, suffix := hashSuffix("password123")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "0000000000000000000000000000000000:3\r\n%s:42\r\n", suffix)
	}))
	defer srv.Close()

	breached, err := newTestHIBPChecker(srv.URL+"/").IsBreached(context.Background(), "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !breached {
		t.Fatal("expected password to be reported as breached")
	}
}

func TestHIBPIsBreached_SuffixNotFound_ReturnsFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "0000000000000000000000000000000000:3\r\n1111111111111111111111111111111111:7\r\n")
	}))
	defer srv.Close()

	breached, err := newTestHIBPChecker(srv.URL+"/").IsBreached(context.Background(), "some-unbreached-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if breached {
		t.Fatal("expected password to not be reported as breached")
	}
}

// A matching suffix with a zero count means the range API still listed the
// entry (rare, but happens for retired entries) — must not be treated as breached.
func TestHIBPIsBreached_SuffixFoundWithZeroCount_ReturnsFalse(t *testing.T) {
	_, suffix := hashSuffix("zero-count-password")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "%s:0\r\n", suffix)
	}))
	defer srv.Close()

	breached, err := newTestHIBPChecker(srv.URL+"/").IsBreached(context.Background(), "zero-count-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if breached {
		t.Fatal("expected a zero-count entry to not be reported as breached")
	}
}

// Transport/5xx failures must fail OPEN — an HIBP outage must not block
// registration/login flows; the caller treats (false, err) as "unknown, proceed".
func TestHIBPIsBreached_ServerError_FailsOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	breached, err := newTestHIBPChecker(srv.URL+"/").IsBreached(context.Background(), "password")
	if err == nil {
		t.Fatal("expected an error on a 5xx response")
	}
	if breached {
		t.Fatal("expected fail-open (false) on a 5xx response")
	}
}

func TestHIBPIsBreached_Timeout_FailsOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	checker := newTestHIBPChecker(srv.URL + "/")
	ctx, cancel := context.WithTimeout(context.Background(), 1)
	defer cancel()

	breached, err := checker.IsBreached(ctx, "password")
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if breached {
		t.Fatal("expected fail-open (false) on timeout")
	}
}

func TestHIBPIsBreached_SendsCorrectPrefixAndPaddingHeader(t *testing.T) {
	prefix, _ := hashSuffix("check-request-shape")

	var gotPath, gotPaddingHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotPaddingHeader = r.Header.Get("Add-Padding")
	}))
	defer srv.Close()

	if _, err := newTestHIBPChecker(srv.URL+"/").IsBreached(context.Background(), "check-request-shape"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/"+prefix {
		t.Errorf("expected request path %q, got %q", "/"+prefix, gotPath)
	}
	if gotPaddingHeader != "true" {
		t.Errorf("expected Add-Padding header to be \"true\", got %q", gotPaddingHeader)
	}
}
