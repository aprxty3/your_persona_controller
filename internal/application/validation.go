package application

import "fmt"

// validStatuses is the exhaustive set of accepted user life-status values.
var validStatuses = map[string]struct{}{
	"student":    {},
	"worker":     {},
	"freelancer": {},
	"unemployed": {},
	"other":      {},
}

// validLocales is the exhaustive set of accepted locale/language codes.
var validLocales = map[string]struct{}{
	"en": {},
	"id": {},
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

// ValidateLocale returns an ErrInvalidInput error if the provided locale
func ValidateLocale(fieldName, locale string) error {
	if locale == "" {
		return fmt.Errorf("%w: %s is required", ErrInvalidInput, fieldName)
	}
	if _, ok := validLocales[locale]; !ok {
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

// ValidateMinLength returns an ErrInvalidInput error if the value is shorter
func ValidateMinLength(fieldName, value string, minLen int) error {
	if len(value) < minLen {
		return fmt.Errorf("%w: %s must be at least %d characters long", ErrInvalidInput, fieldName, minLen)
	}
	return nil
}
