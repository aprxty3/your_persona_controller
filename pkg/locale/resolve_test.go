package locale

import (
	"reflect"
	"testing"
)

func TestResolve_SupportedLocale_ReturnedAsIs(t *testing.T) {
	if got := Resolve("id"); got != "id" {
		t.Errorf("expected id, got %q", got)
	}
	if got := Resolve("en"); got != "en" {
		t.Errorf("expected en, got %q", got)
	}
}

func TestResolve_UnsupportedOrEmpty_FallsBackToEN(t *testing.T) {
	for _, in := range []string{"", "fr", "ID", "EN", "xx"} {
		if got := Resolve(in); got != EN {
			t.Errorf("Resolve(%q) = %q, expected fallback to EN", in, got)
		}
	}
}

func TestIsSupported(t *testing.T) {
	cases := map[string]bool{"en": true, "id": true, "fr": false, "": false, "EN": false}
	for in, want := range cases {
		if got := IsSupported(in); got != want {
			t.Errorf("IsSupported(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseAcceptLanguage_PicksFirstSupportedTag(t *testing.T) {
	got := ParseAcceptLanguage("fr-FR;q=0.9, id-ID;q=0.8, en-US;q=0.7")
	if got != "id" {
		t.Errorf("expected id, got %q", got)
	}
}

func TestParseAcceptLanguage_NoSupportedTag_ReturnsEmpty(t *testing.T) {
	got := ParseAcceptLanguage("fr-FR,de-DE;q=0.9")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestParseAcceptLanguage_EmptyHeader_ReturnsEmpty(t *testing.T) {
	if got := ParseAcceptLanguage(""); got != "" {
		t.Errorf("expected empty string for empty header, got %q", got)
	}
}

func TestParseAcceptLanguage_CaseInsensitiveAndRegionStripped(t *testing.T) {
	got := ParseAcceptLanguage("EN-US")
	if got != "en" {
		t.Errorf("expected en, got %q", got)
	}
}

type localizedItem struct {
	Key    string
	Locale string
}

func TestPickWithFallback_PrefersRequestedLocale(t *testing.T) {
	items := []localizedItem{
		{Key: "greeting", Locale: "en"},
		{Key: "greeting", Locale: "id"},
	}
	got := PickWithFallback(items, func(i localizedItem) string { return i.Key }, func(i localizedItem) string { return i.Locale }, "id")

	want := map[string]localizedItem{"greeting": {Key: "greeting", Locale: "id"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %+v, got %+v", want, got)
	}
}

// A key missing in the requested locale must fall back to its EN variant.
func TestPickWithFallback_MissingInRequestedLocale_FallsBackToEN(t *testing.T) {
	items := []localizedItem{
		{Key: "onlyEN", Locale: "en"},
		{Key: "both", Locale: "en"},
		{Key: "both", Locale: "id"},
	}
	got := PickWithFallback(items, func(i localizedItem) string { return i.Key }, func(i localizedItem) string { return i.Locale }, "id")

	if got["onlyEN"].Locale != "en" {
		t.Errorf("expected onlyEN to fall back to its EN entry, got %+v", got["onlyEN"])
	}
	if got["both"].Locale != "id" {
		t.Errorf("expected both's id entry to win over its EN entry, got %+v", got["both"])
	}
}
