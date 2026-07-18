package content

import "testing"

func tmpl(cond ConditionType, threshold *float64) InsightTemplate {
	return InsightTemplate{ConditionType: cond, ThresholdValue: threshold}
}

func f(v float64) *float64 { return &v }

// MatchesScore is the single threshold rule shared by PDF collectInsights and
// the dashboard evaluator (TICKET-31) — exhaustively covering it here means
// the consumers only need to prove they call it.
func TestMatchesScore(t *testing.T) {
	cases := []struct {
		name string
		tpl  InsightTemplate
		val  float64
		want bool
	}{
		// threshold (>=): first pole, unchanged TICKET-24 semantics
		{"threshold: above matches", tmpl(ConditionThreshold, f(60)), 75, true},
		{"threshold: exactly at boundary matches", tmpl(ConditionThreshold, f(60)), 60, true},
		{"threshold: below does not match", tmpl(ConditionThreshold, f(60)), 59, false},

		// threshold_below (<): second pole (TICKET-31)
		{"below: low score matches", tmpl(ConditionThresholdBelow, f(40)), 30, true},
		{"below: exactly at boundary does not match", tmpl(ConditionThresholdBelow, f(40)), 40, false},
		{"below: high score does not match", tmpl(ConditionThresholdBelow, f(40)), 75, false},

		// neutral band 40-60: neither pole may claim it
		{"neutral zone: no first-pole match", tmpl(ConditionThreshold, f(60)), 50, false},
		{"neutral zone: no second-pole match", tmpl(ConditionThresholdBelow, f(40)), 50, false},

		// score 0 = pre-scoring-engine zero-value, never "extremely second-pole"
		{"below: zero score guarded", tmpl(ConditionThresholdBelow, f(40)), 0, false},

		// robustness
		{"nil threshold never matches", tmpl(ConditionThreshold, nil), 75, false},
		{"delta conditions are not score-matchable", tmpl(ConditionIncrease, f(40)), 30, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tpl.MatchesScore(tc.val); got != tc.want {
				t.Errorf("MatchesScore(%v) = %v, want %v", tc.val, got, tc.want)
			}
		})
	}
}
