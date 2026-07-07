package aivalidator

import (
	"errors"
	"strings"
)

var ErrAIAnomaly = errors.New("AI response contains refusal or anomaly patterns")

// Validate checks the Gemini response against locale-specific refusal patterns.
func Validate(response string, locale string) error {
	lowerResp := strings.ToLower(response)

	// Default English refusal patterns
	refusalPatterns := []string{
		"i cannot fulfill",
		"as an ai",
		"i am unable to",
		"i'm sorry",
	}

	// Append Indonesian patterns if applicable
	if locale == "id" {
		refusalPatterns = append(refusalPatterns, "sebagai ai", "saya tidak dapat", "maaf")
	}

	for _, pattern := range refusalPatterns {
		if strings.Contains(lowerResp, pattern) {
			return ErrAIAnomaly
		}
	}

	// Check for empty or abnormally short responses
	if len(strings.TrimSpace(response)) < 10 {
		return ErrAIAnomaly
	}

	return nil
}
