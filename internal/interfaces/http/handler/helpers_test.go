package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aprxty3/your_persona_controller.git/pkg/httpresponse"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/labstack/echo/v4"
)

func testLog() logger.Logger { return logger.NewLogger("test") }

func newHelperCtx(body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		req = httptest.NewRequest(http.MethodGet, "/", nil)
	}
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestHttpcallError_Writes500WithMessage(t *testing.T) {
	c, rec := newHelperCtx("")
	if err := httpcallError(c, errors.New("boom")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
	var body httpresponse.Response
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Error == nil || body.Error.Code != "INTERNAL_ERROR" || body.Error.Message != "boom" {
		t.Errorf("unexpected error body: %+v", body.Error)
	}
}

func TestHttpcallErrorCustom_WritesGivenCodeAndStatus(t *testing.T) {
	c, rec := newHelperCtx("")
	if err := httpcallErrorCustom(c, http.StatusConflict, "SOME_CODE", "some message"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
	var body httpresponse.Response
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Error == nil || body.Error.Code != "SOME_CODE" {
		t.Errorf("unexpected error body: %+v", body.Error)
	}
}

func TestHttpcallSuccess_WritesDataAndMeta(t *testing.T) {
	c, rec := newHelperCtx("")
	if err := httpcallSuccess(c, http.StatusCreated, map[string]string{"id": "1"}, map[string]int{"total": 5}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
	var body httpresponse.Response
	json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.Success {
		t.Error("expected success=true")
	}
}

func TestBindJSON_ValidBody_PopulatesPayload(t *testing.T) {
	c, _ := newHelperCtx(`{"name":"alice"}`)
	var payload struct {
		Name string `json:"name"`
	}
	if err := bindJSON(c, testLog(), "test action", &payload); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload.Name != "alice" {
		t.Errorf("expected name=alice, got %q", payload.Name)
	}
}

func TestBindJSON_MalformedBody_ReturnsValidationError(t *testing.T) {
	c, rec := newHelperCtx(`{"name":`)
	var payload struct {
		Name string `json:"name"`
	}
	err := bindJSON(c, testLog(), "test action", &payload)
	if !errors.Is(err, errResponseWritten) {
		t.Fatalf("expected errResponseWritten so callers stop processing, got: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	var body httpresponse.Response
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Error == nil || body.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("unexpected error body: %+v", body.Error)
	}
}

// unwrapMessage strips only the FIRST "ctx: " prefix, not every wrap layer.
func TestUnwrapMessage_StripsFirstContextPrefix(t *testing.T) {
	err := errors.New("submit assessment: answer count mismatch")
	if got := unwrapMessage(err); got != "answer count mismatch" {
		t.Errorf("expected the message after the first prefix, got %q", got)
	}
}

func TestUnwrapMessage_NoColonPrefix_ReturnsWholeMessage(t *testing.T) {
	err := errors.New("plain message with no wrapping")
	if got := unwrapMessage(err); got != "plain message with no wrapping" {
		t.Errorf("expected the message unchanged, got %q", got)
	}
}

func TestUnwrapMessage_MultipleWraps_ReturnsInnermostOnly(t *testing.T) {
	err := errors.New("outer: middle: inner detail")
	if got := unwrapMessage(err); got != "middle: inner detail" {
		t.Errorf("expected everything after the FIRST colon-space, got %q", got)
	}
}
