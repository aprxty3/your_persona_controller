package aivalidator

import "strings"

// minLength is the hard floor below which a response is treated as anomalous.
const minLength = 20

// refusalPatterns lists locale-specific phrases that indicate the model declined to answer.
var refusalPatterns = map[string][]string{
	"en": {
		"i cannot fulfill", "i can't fulfill",
		"i cannot assist", "i can't assist",
		"i cannot provide", "i can't provide",
		"i'm not able to", "i am not able to", "i am unable to",
		"as an ai language model", "as an ai, i",
		"i'm sorry, but i can't", "i'm sorry, but i cannot",
	},
	"id": {
		"maaf, saya tidak dapat", "saya tidak bisa membantu",
		"saya tidak dapat memenuhi", "sebagai ai, saya",
	},
}

// ValidateOutput returns an error if text is too short or matches a known refusal pattern for locale.
func ValidateOutput(text string, locale string) error {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < minLength {
		return &ValidationError{Reason: "output_too_short", Detail: trimmed}
	}

	lower := strings.ToLower(trimmed)
	for _, kw := range patternsFor(locale) {
		if strings.Contains(lower, kw) {
			return &ValidationError{Reason: "refusal_pattern_detected", Detail: kw}
		}
	}

	return nil
}

// patternsFor returns the refusal phrase list for locale.
func patternsFor(locale string) []string {
	patterns := refusalPatterns["en"]
	if locale != "en" {
		if extra, ok := refusalPatterns[locale]; ok {
			combined := make([]string, 0, len(patterns)+len(extra))
			combined = append(combined, patterns...)
			combined = append(combined, extra...)
			return combined
		}
	}
	return patterns
}

// ValidationError describes why Gemini output was rejected.
type ValidationError struct {
	Reason string
	Detail string
}

func (e *ValidationError) Error() string {
	return "aivalidator: " + e.Reason + ": " + e.Detail
}
