package i18n

import "testing"

func TestLoadCatalog_LoadsBothLocales(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := catalog.Message("otp_verification", "en"); !ok {
		t.Fatal("expected otp_verification to load for en")
	}
	if _, ok := catalog.Message("otp_verification", "id"); !ok {
		t.Fatal("expected otp_verification to load for id")
	}
}

func TestCatalog_Message_ResolvesRequestedLocale(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg, ok := catalog.Message("otp_verification", "id")
	if !ok {
		t.Fatal("expected otp_verification/id to be found")
	}
	if msg.Subject != "Your Persona's - Kode Verifikasi" {
		t.Fatalf("expected Indonesian subject, got %q", msg.Subject)
	}
}

// FR-I9: unsupported/unset locale must fall back to EN content, not error out.
func TestCatalog_Message_UnsupportedLocaleFallsBackToEN(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	frMsg, ok := catalog.Message("otp_verification", "fr")
	if !ok {
		t.Fatal("expected fallback to EN content for an unsupported locale")
	}
	enMsg, _ := catalog.Message("otp_verification", "en")
	if frMsg != enMsg {
		t.Fatalf("expected fr to resolve to identical EN content, got %+v vs %+v", frMsg, enMsg)
	}

	emptyMsg, ok := catalog.Message("otp_verification", "")
	if !ok || emptyMsg != enMsg {
		t.Fatalf("expected empty locale to resolve to EN content too, got ok=%v msg=%+v", ok, emptyMsg)
	}
}

func TestCatalog_Message_UnknownPurpose_NotOK(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := catalog.Message("no_such_purpose", "en"); ok {
		t.Fatal("expected ok=false for an unknown purpose")
	}
}
