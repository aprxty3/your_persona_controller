package content

// ConditionType defines how a micro-insight is triggered.
type ConditionType string

const (
	ConditionIncrease       ConditionType = "increase"
	ConditionDecrease       ConditionType = "decrease"
	ConditionThreshold      ConditionType = "threshold"
	ConditionThresholdBelow ConditionType = "threshold_below"
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

// MatchesScore reports whether a threshold-family template fires for the
// given trait score — the single evaluation rule shared by both consumers
// (PDF collectInsights and the dashboard micro-insight evaluator), so the
// two can never drift on threshold semantics. Delta-family conditions
// (increase/decrease) return false here: they compare TWO results and are
// evaluated where a previous result exists (dashboard only).
func (t InsightTemplate) MatchesScore(value float64) bool {
	if t.ThresholdValue == nil {
		return false
	}
	switch t.ConditionType {
	case ConditionThreshold:
		return value >= *t.ThresholdValue
	case ConditionThresholdBelow:
		return value > 0 && value < *t.ThresholdValue
	default:
		return false
	}
}
