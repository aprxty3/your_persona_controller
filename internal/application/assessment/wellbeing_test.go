package assessment

import "testing"

func TestScanForCrisisLanguage_EnglishKeyword_Detected(t *testing.T) {
	if !scanForCrisisLanguage([]string{"Sometimes I want to end my life."}, "en") {
		t.Fatal("expected an EN crisis keyword to be detected")
	}
}

func TestScanForCrisisLanguage_CaseInsensitive(t *testing.T) {
	if !scanForCrisisLanguage([]string{"I WANT TO DIE."}, "en") {
		t.Fatal("expected keyword matching to be case-insensitive")
	}
}

func TestScanForCrisisLanguage_IndonesianKeyword_Detected(t *testing.T) {
	if !scanForCrisisLanguage([]string{"Aku sering berpikir untuk bunuh diri."}, "id") {
		t.Fatal("expected an ID crisis keyword to be detected")
	}
}

// Non-EN locales still check EN keywords too (defense in depth against
// code-switching essays).
func TestScanForCrisisLanguage_IDLocale_StillMatchesEnglishKeyword(t *testing.T) {
	if !scanForCrisisLanguage([]string{"I want to kill myself"}, "id") {
		t.Fatal("expected EN keywords to still be checked under an ID locale")
	}
}

func TestScanForCrisisLanguage_NoMatch_False(t *testing.T) {
	if scanForCrisisLanguage([]string{"I had a great day today, feeling motivated."}, "en") {
		t.Fatal("expected no crisis flag for benign text")
	}
}

func TestScanForCrisisLanguage_EmptyTexts_False(t *testing.T) {
	if scanForCrisisLanguage(nil, "en") {
		t.Fatal("expected no crisis flag for an empty essay set")
	}
}

func TestFallbackText_KnownLocale(t *testing.T) {
	if fallbackText("id") == fallbackText("en") {
		t.Fatal("expected id and en fallback text to differ")
	}
}

func TestFallbackText_UnknownLocale_FallsBackToEN(t *testing.T) {
	if fallbackText("fr") != fallbackText("en") {
		t.Fatal("expected an unsupported locale to fall back to EN text")
	}
}
