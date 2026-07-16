package aivalidator

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateOutput_ValidText_ReturnsNil(t *testing.T) {
	err := ValidateOutput(strings.Repeat("a", 50), "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateOutput_TooShort_ReturnsError(t *testing.T) {
	err := ValidateOutput("too short", "en")
	if err == nil {
		t.Fatal("expected an error for text under minLength")
	}
	var vErr *ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("expected a *ValidationError, got %T", err)
	}
	if vErr.Reason != "output_too_short" {
		t.Errorf("expected reason output_too_short, got %q", vErr.Reason)
	}
}

func TestValidateOutput_WhitespaceOnly_TreatedAsTooShort(t *testing.T) {
	err := ValidateOutput("   \n\t   ", "en")
	if err == nil {
		t.Fatal("expected an error for whitespace-only text")
	}
}

func TestValidateOutput_EnglishRefusalPattern_Detected(t *testing.T) {
	text := "I'm sorry, but I cannot fulfill this request as it goes beyond my capabilities today."
	err := ValidateOutput(text, "en")
	if err == nil {
		t.Fatal("expected a refusal pattern to be detected")
	}
	var vErr *ValidationError
	if !errors.As(err, &vErr) || vErr.Reason != "refusal_pattern_detected" {
		t.Fatalf("expected refusal_pattern_detected, got %v", err)
	}
}

func TestValidateOutput_RefusalPattern_CaseInsensitive(t *testing.T) {
	text := "I CANNOT ASSIST with that particular request due to policy constraints today."
	if err := ValidateOutput(text, "en"); err == nil {
		t.Fatal("expected the refusal pattern check to be case-insensitive")
	}
}

func TestValidateOutput_IndonesianRefusalPattern_Detected(t *testing.T) {
	text := "Maaf, saya tidak dapat membantu permintaan ini karena melanggar kebijakan yang berlaku."
	err := ValidateOutput(text, "id")
	if err == nil {
		t.Fatal("expected an Indonesian refusal pattern to be detected")
	}
}

// Non-EN locales must still catch EN refusal phrases — patternsFor combines
// the EN baseline with any locale-specific additions, it never replaces it.
func TestValidateOutput_IDLocale_StillCatchesENPhrases(t *testing.T) {
	text := "I'm sorry, but I cannot fulfill this request due to safety guidelines in place."
	err := ValidateOutput(text, "id")
	if err == nil {
		t.Fatal("expected the EN refusal phrase to still be caught under the 'id' locale")
	}
}

func TestValidateOutput_UnknownLocale_FallsBackToEN(t *testing.T) {
	text := "I cannot provide that information due to the nature of this particular request today."
	err := ValidateOutput(text, "fr")
	if err == nil {
		t.Fatal("expected an unknown locale to still apply the EN refusal patterns")
	}
}

func TestValidateOutput_LegitimateLongText_NoFalsePositive(t *testing.T) {
	text := "The user demonstrates strong conscientiousness traits and shows a clear preference for structured environments over ambiguous ones."
	if err := ValidateOutput(text, "en"); err != nil {
		t.Fatalf("expected no false positive, got: %v", err)
	}
}

func TestValidationError_Error_IncludesReasonAndDetail(t *testing.T) {
	err := &ValidationError{Reason: "output_too_short", Detail: "hi"}
	msg := err.Error()
	if !strings.Contains(msg, "output_too_short") || !strings.Contains(msg, "hi") {
		t.Errorf("expected error message to include reason and detail, got: %s", msg)
	}
}
