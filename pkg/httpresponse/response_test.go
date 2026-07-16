package httpresponse

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func newTestContext() (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestSuccess_WritesEnvelopeWithData(t *testing.T) {
	c, rec := newTestContext()

	if err := Success(c, http.StatusOK, map[string]string{"foo": "bar"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body Response
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal response body: %v", err)
	}
	if !body.Success {
		t.Error("expected success=true")
	}
	if body.Error != nil {
		t.Errorf("expected no error field, got %+v", body.Error)
	}
	data, ok := body.Data.(map[string]interface{})
	if !ok || data["foo"] != "bar" {
		t.Errorf("expected data.foo=bar, got %+v", body.Data)
	}
}

func TestSuccess_IncludesMetaWhenProvided(t *testing.T) {
	c, rec := newTestContext()

	if err := Success(c, http.StatusOK, []int{1, 2, 3}, map[string]int{"page": 1}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body Response
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	meta, ok := body.Meta.(map[string]interface{})
	if !ok || meta["page"] != float64(1) {
		t.Errorf("expected meta.page=1, got %+v", body.Meta)
	}
}

func TestError_WritesEnvelopeWithErrorDetail(t *testing.T) {
	c, rec := newTestContext()

	if err := Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "bad input"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var body Response
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if body.Success {
		t.Error("expected success=false")
	}
	if body.Data != nil {
		t.Errorf("expected no data field, got %+v", body.Data)
	}
	if body.Error == nil || body.Error.Code != "VALIDATION_ERROR" || body.Error.Message != "bad input" {
		t.Errorf("expected error={VALIDATION_ERROR, bad input}, got %+v", body.Error)
	}
}

func TestResponse_OmitsEmptyFieldsInJSON(t *testing.T) {
	c, rec := newTestContext()

	if err := Success(c, http.StatusOK, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := rec.Body.String()
	for _, unexpected := range []string{`"data"`, `"meta"`, `"error"`} {
		if strings.Contains(body, unexpected) {
			t.Errorf("expected omitempty fields to be absent from JSON, found %q in: %s", unexpected, body)
		}
	}
}
