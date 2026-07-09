package insighttemplate

import "context"

// Repository defines the contract for InsightTemplate data persistence.
type Repository interface {
	// FindMatchingTemplates returns active templates for a given trait and locale
	// that match the provided delta or score value. Falls back to "en" if the
	// requested locale is not found (FR-I9).
	FindMatchingTemplates(ctx context.Context, trait, locale string) ([]InsightTemplate, error)
}
