package gemini

import (
	"context"
	"strings"
	"testing"
)

// An empty apiKey leaves client.client nil (rather than erroring at
// construction) — GenerateSummary must reject that state up front instead
// of dereferencing a nil SDK client.
func TestNewClient_EmptyAPIKey_LeavesClientUnconfigured(t *testing.T) {
	c, err := NewClient("", "gemini-2.0-flash", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected a non-nil Client wrapper even when unconfigured")
	}
	if c.client != nil {
		t.Fatal("expected the underlying SDK client to be nil when apiKey is empty")
	}
}

func TestGenerateSummary_UnconfiguredClient_ReturnsError(t *testing.T) {
	c, err := NewClient("", "gemini-2.0-flash", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	summary, rawPrompt, tokens, err := c.GenerateSummary(context.Background(), "my essay text", "en")
	if err == nil {
		t.Fatal("expected an error when the Gemini client is unconfigured")
	}
	if summary != "" {
		t.Errorf("expected an empty summary on error, got %q", summary)
	}
	if tokens != 0 {
		t.Errorf("expected 0 tokens on error, got %d", tokens)
	}
	if !strings.Contains(rawPrompt, "my essay text") {
		t.Errorf("expected rawPrompt to still be built (for audit logging) even on the unconfigured-client error path, got: %s", rawPrompt)
	}
}

// rawPrompt is persisted for prompt-audit purposes even when the call fails,
// so its shape (locale-aware system instruction + essay body) must be
// exercised directly.
func TestGenerateSummary_RawPromptIncludesLocaleAndEssay(t *testing.T) {
	c, err := NewClient("", "gemini-2.0-flash", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, rawPrompt, _, _ := c.GenerateSummary(context.Background(), "essay about grit", "id")
	if !strings.Contains(rawPrompt, "'id' language") {
		t.Errorf("expected rawPrompt to mention the requested locale, got: %s", rawPrompt)
	}
	if !strings.Contains(rawPrompt, "essay about grit") {
		t.Errorf("expected rawPrompt to contain the essay text, got: %s", rawPrompt)
	}
	if !strings.Contains(rawPrompt, "GRIT and MBTI") {
		t.Errorf("expected rawPrompt to mention the GRIT/MBTI focus instruction, got: %s", rawPrompt)
	}
}

func TestClose_UnconfiguredClient_DoesNotPanic(t *testing.T) {
	c, err := NewClient("", "gemini-2.0-flash", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c.Close()
}
