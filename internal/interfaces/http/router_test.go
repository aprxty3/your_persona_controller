package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/labstack/echo/v4"
)

func TestParseAllowedOrigins_TrimsAndSplits(t *testing.T) {
	got := ParseAllowedOrigins("https://a.com, https://b.com")
	if len(got) != 2 || got[0] != "https://a.com" || got[1] != "https://b.com" {
		t.Fatalf("expected 2 trimmed origins, got %v", got)
	}
}

func TestParseAllowedOrigins_EmptyString_NilSlice(t *testing.T) {
	got := ParseAllowedOrigins("")
	if got != nil {
		t.Fatalf("expected nil slice for empty input, got %v", got)
	}
}

func TestParseAllowedOrigins_SkipsEmptyEntries(t *testing.T) {
	got := ParseAllowedOrigins("https://a.com,,  ,https://b.com")
	if len(got) != 2 {
		t.Fatalf("expected empty entries to be skipped, got %v", got)
	}
}

// Security contract: a bare "*" origin must panic at startup, not
// silently pass through as an allowed origin — enforced structurally, not
// just documented.
func TestParseAllowedOrigins_WildcardOnly_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on a literal \"*\" origin")
		}
	}()
	ParseAllowedOrigins("*")
}

func TestParseAllowedOrigins_WildcardAmongOthers_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when \"*\" appears among other origins")
		}
	}()
	ParseAllowedOrigins("https://a.com,*")
}

func TestErrorCodeForStatus_MapsFrameworkStatuses(t *testing.T) {
	cases := map[int]string{
		http.StatusBadRequest:            "BAD_REQUEST",
		http.StatusUnauthorized:          "UNAUTHORIZED",
		http.StatusForbidden:             "FORBIDDEN",
		http.StatusNotFound:              "NOT_FOUND",
		http.StatusMethodNotAllowed:      "METHOD_NOT_ALLOWED",
		http.StatusRequestEntityTooLarge: "PAYLOAD_TOO_LARGE",
		http.StatusTooManyRequests:       "RATE_LIMITED",
		http.StatusBadGateway:            "INTERNAL_ERROR",
		http.StatusTeapot:                "BAD_REQUEST",
	}
	for status, want := range cases {
		if got := errorCodeForStatus(status); got != want {
			t.Errorf("status %d: got %q, want %q", status, got, want)
		}
	}
}

// A framework-level rejection (CSRF, 404, 413 …) must come back in the same
// envelope handlers use. Echo's default handler emits {"message": "..."},
// which the front-end's envelope parser cannot read — it turns every one of
// them into an opaque "Unexpected response (HTTP nnn)".
func TestHTTPErrorHandler_WritesEnvelope(t *testing.T) {
	e := echo.New()
	installErrorHandler(e, logger.NewLogger("test"))

	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodPost, "/v1/account/profile", nil), rec)
	e.HTTPErrorHandler(echo.NewHTTPError(http.StatusBadRequest, "missing csrf token in request header"), c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", rec.Code)
	}
	var body struct {
		Success bool `json:"success"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, rec.Body.String())
	}
	if body.Success {
		t.Error("success: got true, want false")
	}
	if body.Error.Code != "BAD_REQUEST" {
		t.Errorf("error.code: got %q, want BAD_REQUEST", body.Error.Code)
	}
	if body.Error.Message != "missing csrf token in request header" {
		t.Errorf("error.message: got %q, want the original reason", body.Error.Message)
	}
}

// 5xx must never carry the internal reason to the client.
func TestHTTPErrorHandler_HidesServerErrorDetail(t *testing.T) {
	e := echo.New()
	installErrorHandler(e, logger.NewLogger("test"))

	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/v1/questions", nil), rec)
	e.HTTPErrorHandler(echo.NewHTTPError(http.StatusInternalServerError, "pq: relation \"users\" does not exist"), c)

	if got := rec.Body.String(); strings.Contains(got, "relation") {
		t.Errorf("internal detail leaked to client: %s", got)
	}
}
