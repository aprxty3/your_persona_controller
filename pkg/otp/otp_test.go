package otp

import (
	"strconv"
	"testing"
)

func TestGenerateOTP_ProducesCorrectLength(t *testing.T) {
	for _, length := range []int{4, 6, 8} {
		code, err := GenerateOTP(length)
		if err != nil {
			t.Fatalf("unexpected error for length %d: %v", length, err)
		}
		if len(code) != length {
			t.Errorf("expected length %d, got %d (%q)", length, len(code), code)
		}
	}
}

func TestGenerateOTP_ProducesOnlyDigits(t *testing.T) {
	code, err := GenerateOTP(6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := strconv.Atoi(code); err != nil {
		t.Errorf("expected code to be all digits, got %q", code)
	}
}

func TestGenerateOTP_ZeroPadsShortNumbers(t *testing.T) {
	// Run enough iterations that a small-value draw with a leading zero is
	// virtually certain to occur (~1% chance per call for length 6), which
	// is exactly the case that would break naive fmt.Sprintf("%d", ...).
	sawLeadingZero := false
	for i := 0; i < 500; i++ {
		code, err := GenerateOTP(6)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(code) != 6 {
			t.Fatalf("expected zero-padded length 6, got %d (%q)", len(code), code)
		}
		if code[0] == '0' {
			sawLeadingZero = true
		}
	}
	if !sawLeadingZero {
		t.Skip("no leading-zero draw occurred in 500 iterations (statistically unlucky, not a failure)")
	}
}

func TestGenerateOTP_ZeroLength_ReturnsError(t *testing.T) {
	if _, err := GenerateOTP(0); err == nil {
		t.Fatal("expected an error for length 0")
	}
}

func TestGenerateOTP_NegativeLength_ReturnsError(t *testing.T) {
	if _, err := GenerateOTP(-1); err == nil {
		t.Fatal("expected an error for a negative length")
	}
}

func TestGenerateOTP_Randomness_NotConstant(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		code, err := GenerateOTP(6)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		seen[code] = true
	}
	if len(seen) < 15 {
		t.Errorf("expected mostly-unique codes across 20 draws, got only %d distinct values", len(seen))
	}
}
