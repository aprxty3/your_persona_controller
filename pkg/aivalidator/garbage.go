package aivalidator

import (
	"strings"
	"unicode"
)

// Thresholds for IsGarbage — named and gathered here so recalibration is a
// one-line change, not a hunt through the function body.
const (
	garbageMinLength           = 30   // trimmed text shorter than this can't say anything analyzable
	garbageUniqueRatioMinLen   = 50   // uniqueness ratio is noisy on very short strings; only trust it above this length
	garbageMinUniqueRuneRatio  = 0.15 // "aaaa...", "hahaha..." collapse to a handful of distinct runes
	garbageMaxNonLetterRatio   = 0.5  // symbol/number mash ("!!!111!!!") is mostly non-letters
	garbageMaxUnbrokenWordSize = 40   // one giant token with no spaces ("asdfghjkl...") isn't prose
)

// IsGarbage reports whether text is not worth spending a Gemini call to
// analyze — too short, too repetitive, too symbol-heavy, or one unbroken
// keyboard mash. This is a soft signal only: callers still accept the
// submission and store the answer verbatim, they just skip sending this
// particular essay to the AI (see SubmitAssessmentUseCase's existing
// no-essay fallback_static path, which this reuses).
func IsGarbage(text string) bool {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < garbageMinLength {
		return true
	}

	runes := []rune(trimmed)
	if len(runes) >= garbageUniqueRatioMinLen && uniqueRuneRatio(runes) < garbageMinUniqueRuneRatio {
		return true
	}

	if nonLetterRatio(runes) > garbageMaxNonLetterRatio {
		return true
	}

	if longestWordLength(trimmed) > garbageMaxUnbrokenWordSize {
		return true
	}

	return false
}

// uniqueRuneRatio is the fraction of runes in text that are distinct —
// low values catch repeated-character spam like "aaaaaaaa" or "hahahaha".
func uniqueRuneRatio(runes []rune) float64 {
	seen := make(map[rune]struct{}, len(runes))
	for _, r := range runes {
		seen[r] = struct{}{}
	}
	return float64(len(seen)) / float64(len(runes))
}

// nonLetterRatio is the fraction of non-whitespace runes that are NOT
// letters — whitespace is excluded from both numerator and denominator so
// normal sentence spacing never counts against legit prose; only symbol/digit
// density does.
func nonLetterRatio(runes []rune) float64 {
	var total, nonLetter int
	for _, r := range runes {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if !unicode.IsLetter(r) {
			nonLetter++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(nonLetter) / float64(total)
}

// longestWordLength returns the rune-length of the longest whitespace-delimited token in text.
func longestWordLength(text string) int {
	max := 0
	for _, word := range strings.Fields(text) {
		if n := len([]rune(word)); n > max {
			max = n
		}
	}
	return max
}
