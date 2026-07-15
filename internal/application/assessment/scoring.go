package assessment

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/content"
)

// mbtiDimensions lists the four MBTI axes in canonical output order. Each
// two-letter code reads "first pole / second pole" — positive scoring
// contributions lean toward the first pole (E/S/T/J), negative toward the
// second (I/N/F/P).
var mbtiDimensions = [4]string{"EI", "SN", "TF", "JP"}

const (
	// TraitGrit is the trait code marking Likert items that feed the GRIT
	// score. GRIT is scored as a plain 1-5 scale average, never mixed with
	// the signed MBTI point system.
	TraitGrit = "GRIT"

	// likertMaxContribution is the largest |effective value - neutral| a
	// single Likert item can contribute to an MBTI dimension (|5-3| = 2).
	likertMaxContribution = 2

	// neutralScore is the fallback percentage for a dimension with no valid
	// answers — dead center, first pole by the >= 50 rule.
	neutralScore = 50
)

// ScoreResult carries everything ComputeScores derives from one answer set.
type ScoreResult struct {
	MBTIType    string
	GritScore   int
	TraitScores map[string]interface{}
	// NeutralFallbackDimensions lists dimensions that had no valid answer and
	// fell back to neutralScore. Callers should Warn-log these — a non-empty
	// list on a full submission means the question bank metadata is broken.
	NeutralFallbackDimensions []string
}

// ComputeScores derives the MBTI type, GRIT score (0-100), and per-dimension
// trait scores from a submitted answer set plus question metadata. Pure
// function — no DB, no side effects; malformed answers or metadata degrade to
// zero contribution instead of failing the submission.
//
// Formula: per MBTI dimension, Likert items contribute
// (effectiveValue - 3) and SJT options contribute their signed
// option_trait_map points; percentage = round(50 + sum/maxAbs*50) clamped to
// 0-100, letter = first pole when >= 50. GRIT = round((avgEffective-1)/4*100)
// over GRIT-trait Likert items only.
func ComputeScores(answers []AnswerInput, questions map[string]content.Question) ScoreResult {
	sums := make(map[string]float64)
	maxAbs := make(map[string]float64)
	var gritValues []float64

	for _, a := range answers {
		q, ok := questions[a.QuestionID]
		if !ok || q.IsAttentionCheck {
			continue
		}
		switch q.Type {
		case content.TypeLikert:
			scoreLikertAnswer(a, q, sums, maxAbs, &gritValues)
		case content.TypeMultipleChoice:
			scoreSJTAnswer(a, q, sums, maxAbs)
		}
	}

	res := ScoreResult{TraitScores: make(map[string]interface{}, len(mbtiDimensions)+1)}

	var mbti strings.Builder
	for _, dim := range mbtiDimensions {
		score := neutralScore
		if maxAbs[dim] > 0 {
			score = clampPercent(math.Round(50 + sums[dim]/maxAbs[dim]*50))
		} else {
			res.NeutralFallbackDimensions = append(res.NeutralFallbackDimensions, dim)
		}
		res.TraitScores[dim] = score
		if score >= 50 {
			mbti.WriteByte(dim[0])
		} else {
			mbti.WriteByte(dim[1])
		}
	}
	res.MBTIType = mbti.String()

	res.GritScore = neutralScore
	if len(gritValues) > 0 {
		var sum float64
		for _, v := range gritValues {
			sum += v
		}
		avg := sum / float64(len(gritValues))
		res.GritScore = clampPercent(math.Round((avg - 1) / 4 * 100))
	} else {
		res.NeutralFallbackDimensions = append(res.NeutralFallbackDimensions, TraitGrit)
	}
	res.TraitScores[TraitGrit] = res.GritScore

	return res
}

// scoreLikertAnswer folds one Likert answer into the running tallies. GRIT
// items are collected separately for the scale-average formula; every other
// trait contributes signed points around the neutral midpoint 3.
func scoreLikertAnswer(a AnswerInput, q content.Question, sums, maxAbs map[string]float64, gritValues *[]float64) {
	if q.Trait == "" {
		return
	}
	v, err := strconv.Atoi(strings.TrimSpace(a.Value))
	if err != nil || v < 1 || v > 5 {
		return
	}

	effective := float64(v)
	if q.IsReverseScored {
		effective = 6 - effective
	}

	if q.Trait == TraitGrit {
		*gritValues = append(*gritValues, effective)
		return
	}
	sums[q.Trait] += effective - 3
	maxAbs[q.Trait] += likertMaxContribution
}

// scoreSJTAnswer folds one SJT answer into the running tallies. An answered
// question widens the denominator of every dimension ANY of its options could
// move — picking an option that doesn't touch a dimension is an implicit
// neutral on it, not a smaller scale. Unknown option letters or unparsable
// maps are ignored (contribution 0).
func scoreSJTAnswer(a AnswerInput, q content.Question, sums, maxAbs map[string]float64) {
	if q.OptionTraitMap == nil {
		return
	}
	var optionPoints map[string]map[string]float64
	if err := json.Unmarshal([]byte(*q.OptionTraitMap), &optionPoints); err != nil {
		return
	}

	chosen, answered := optionPoints[strings.ToUpper(strings.TrimSpace(a.Value))]
	if !answered {
		return
	}

	// Denominator: the largest |points| among this question's options, per dimension.
	perDimMax := make(map[string]float64)
	for _, points := range optionPoints {
		for dim, v := range points {
			if abs := math.Abs(v); abs > perDimMax[dim] {
				perDimMax[dim] = abs
			}
		}
	}
	for dim, m := range perDimMax {
		maxAbs[dim] += m
	}
	for dim, v := range chosen {
		sums[dim] += v
	}
}

// clampPercent bounds a rounded percentage to the 0-100 display range.
func clampPercent(v float64) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return int(v)
}
