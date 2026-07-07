package insighttemplate

import "context"

// ConditionType defines how a micro-insight is triggered.
type ConditionType string

const (
	// ConditionIncrease triggers when a trait score increases by at least MinDelta.
	ConditionIncrease ConditionType = "increase"

	// ConditionDecrease triggers when a trait score decreases by at least MinDelta.
	ConditionDecrease ConditionType = "decrease"

	// ConditionThreshold triggers when a trait score meets or exceeds ThresholdValue.
	ConditionThreshold ConditionType = "threshold"
)

// InsightTemplate is a rule-based micro-insight definition used in the Member dashboard.
// Insights are generated without a Gemini call — purely from templates + delta math (FR-F4).
//
// Composite UNIQUE constraint: (insight_key, locale) — see ERD.
// The template_text supports placeholders, e.g. "{trait} score went up by {delta} points".
//
// Per PRD Section 3a: every numeric display MUST be accompanied by qualitative framing.
// These templates provide that framing for the GRIT trend line and trait deltas.
type InsightTemplate struct {
	ID             string
	InsightKey     string        // e.g. "grit_increase_high" — unique per locale pair
	Locale         string        // "en", "id", etc.
	Trait          string        // "grit" | "E" | "I" | "S" | "N" | "T" | "F" | "J" | "P"
	ConditionType  ConditionType
	MinDelta       *float64 // used for increase/decrease; nil for threshold
	ThresholdValue *float64 // used for threshold; nil for increase/decrease
	TemplateText   string   // supports {trait}, {delta} placeholders
	IsActive       bool
}

// Repository defines the contract for InsightTemplate data persistence.
type Repository interface {
	// FindMatchingTemplates returns active templates for a given trait and locale
	// that match the provided delta or score value. Falls back to "en" if the
	// requested locale is not found (FR-I9).
	FindMatchingTemplates(ctx context.Context, trait, locale string) ([]InsightTemplate, error)
}
