package http

import "testing"

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
