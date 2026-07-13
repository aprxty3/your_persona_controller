package content

// ConditionType defines how a micro-insight is triggered.
type ConditionType string

const (
	ConditionIncrease  ConditionType = "increase"
	ConditionDecrease  ConditionType = "decrease"
	ConditionThreshold ConditionType = "threshold"
)

// InsightTemplate is a rule-based micro-insight definition used in the Member dashboard.
type InsightTemplate struct {
	ID             string
	InsightKey     string
	Locale         string
	Trait          string
	ConditionType  ConditionType
	MinDelta       *float64
	ThresholdValue *float64
	TemplateText   string
	IsActive       bool
}
