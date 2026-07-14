package application

import (
	"fmt"
	"regexp"

	pkglocale "github.com/aprxty3/your_persona_controller.git/pkg/locale"
)

// emailPattern is a deliberately loose shape check (not full RFC 5322).
var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// validStatuses is the exhaustive set of accepted user life-status values.
var validStatuses = map[string]struct{}{
	"student":    {},
	"worker":     {},
	"freelancer": {},
	"unemployed": {},
	"other":      {},
}

// ValidateStatus returns an ErrInvalidInput error if the provided status
func ValidateStatus(status string) error {
	if status == "" {
		return fmt.Errorf("%w: status is required", ErrInvalidInput)
	}
	if _, ok := validStatuses[status]; !ok {
		return fmt.Errorf("%w: status must be one of: student, worker, freelancer, unemployed, other", ErrInvalidInput)
	}
	return nil
}

// ValidateLocale returns an ErrInvalidInput error if the provided locale/language
// code isn't one of the MVP-supported locales — delegates to pkg/locale.IsSupported
// so this and the HTTP-layer locale negotiation (LocaleMiddleware) never drift
// out of sync on what "supported" means.
func ValidateLocale(fieldName, code string) error {
	if code == "" {
		return fmt.Errorf("%w: %s is required", ErrInvalidInput, fieldName)
	}
	if !pkglocale.IsSupported(code) {
		return fmt.Errorf("%w: %s must be one of: en, id", ErrInvalidInput, fieldName)
	}
	return nil
}

// ValidateAge returns an ErrInvalidInput error if the age is below minimum allowed value
func ValidateAge(age, minAge int) error {
	if age < minAge {
		return fmt.Errorf("%w: age must be at least %d", ErrInvalidInput, minAge)
	}
	return nil
}

// ValidateRequired returns an ErrInvalidInput error if the value is an empty string.
func ValidateRequired(fieldName, value string) error {
	if value == "" {
		return fmt.Errorf("%w: %s is required", ErrInvalidInput, fieldName)
	}
	return nil
}

// ValidateEmail returns an ErrInvalidInput error if the value is empty.
func ValidateEmail(fieldName, value string) error {
	if err := ValidateRequired(fieldName, value); err != nil {
		return err
	}
	if !emailPattern.MatchString(value) {
		return fmt.Errorf("%w: %s must be a valid email address", ErrInvalidInput, fieldName)
	}
	return nil
}

// ValidateMinLength returns an ErrInvalidInput error if the value is shorter
func ValidateMinLength(fieldName, value string, minLen int) error {
	if len(value) < minLen {
		return fmt.Errorf("%w: %s must be at least %d characters long", ErrInvalidInput, fieldName, minLen)
	}
	return nil
}

// ValidateMaxLength returns an ErrInvalidInput error if the value is longer than maxLen characters.
func ValidateMaxLength(fieldName, value string, maxLen int) error {
	if len(value) > maxLen {
		return fmt.Errorf("%w: %s must not exceed %d characters", ErrInvalidInput, fieldName, maxLen)
	}
	return nil
}
