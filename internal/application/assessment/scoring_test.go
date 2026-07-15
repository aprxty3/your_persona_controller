package assessment

import (
	"testing"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/content"
)

// Fixture IDs mirror cmd/seed/main.go so the worked examples in TICKET-17
// stay readable against the real question bank.
const (
	qSJT1     = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"
	qSJT2     = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12"
	qSJT3     = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a13"
	qLikertEI = "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b11"
	qLikertSN = "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b12"
	qLikertTF = "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b13"
	qGritRev1 = "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b14"
	qLikertJP = "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b15"
	qSNRev    = "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b16"
	qAttn     = "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b17"
	qGrit     = "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b18"
	qGritRev2 = "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b19"
	qGritRev3 = "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b20"
)

func seedQuestionFixtures() map[string]content.Question {
	sjtMap1 := `{"A":{"EI":2},"B":{"EI":-2},"C":{"TF":-1},"D":{"EI":-1},"E":{}}`
	sjtMap2 := `{"A":{"JP":1},"B":{"TF":1},"C":{"EI":1},"D":{"JP":-1},"E":{}}`
	sjtMap3 := `{"A":{"TF":1,"EI":1},"B":{},"C":{"TF":-1},"D":{"JP":1},"E":{}}`

	return map[string]content.Question{
		qSJT1:     {ID: qSJT1, Type: content.TypeMultipleChoice, OptionTraitMap: &sjtMap1},
		qSJT2:     {ID: qSJT2, Type: content.TypeMultipleChoice, OptionTraitMap: &sjtMap2},
		qSJT3:     {ID: qSJT3, Type: content.TypeMultipleChoice, OptionTraitMap: &sjtMap3},
		qLikertEI: {ID: qLikertEI, Type: content.TypeLikert, Trait: "EI"},
		qLikertSN: {ID: qLikertSN, Type: content.TypeLikert, Trait: "SN"},
		qLikertTF: {ID: qLikertTF, Type: content.TypeLikert, Trait: "TF"},
		qGritRev1: {ID: qGritRev1, Type: content.TypeLikert, Trait: "GRIT", IsReverseScored: true},
		qLikertJP: {ID: qLikertJP, Type: content.TypeLikert, Trait: "JP"},
		qSNRev:    {ID: qSNRev, Type: content.TypeLikert, Trait: "SN", IsReverseScored: true},
		qAttn:     {ID: qAttn, Type: content.TypeLikert, IsAttentionCheck: true},
		qGrit:     {ID: qGrit, Type: content.TypeLikert, Trait: "GRIT"},
		qGritRev2: {ID: qGritRev2, Type: content.TypeLikert, Trait: "GRIT", IsReverseScored: true},
		qGritRev3: {ID: qGritRev3, Type: content.TypeLikert, Trait: "GRIT", IsReverseScored: true},
	}
}

// TestComputeScores_WorkedExampleEI reproduces  numeric contract:
// Likert b11=4 (+1), SJT a11="B" (-2), a12="C" (+1), a13="A" (+1) →
// sum=+1, maxAbs=2+2+1+1=6 → round(50 + 1/6*50) = 58 → letter E.
func TestComputeScores_WorkedExampleEI(t *testing.T) {
	answers := []AnswerInput{
		{QuestionID: qLikertEI, Value: "4"},
		{QuestionID: qSJT1, Value: "B"},
		{QuestionID: qSJT2, Value: "C"},
		{QuestionID: qSJT3, Value: "A"},
	}

	res := ComputeScores(answers, seedQuestionFixtures())

	if got := res.TraitScores["EI"]; got != 58 {
		t.Errorf("EI score = %v, want 58", got)
	}
	if res.MBTIType[0] != 'E' {
		t.Errorf("MBTI first letter = %c, want E", res.MBTIType[0])
	}
}

// TestComputeScores_WorkedExampleGrit reproduces numeric contract:
// b14=2 (rev→4), b18=5, b19=2 (rev→4), b20=1 (rev→5) → avg 4.5 →
// round(3.5/4*100) = 88.
func TestComputeScores_WorkedExampleGrit(t *testing.T) {
	answers := []AnswerInput{
		{QuestionID: qGritRev1, Value: "2"},
		{QuestionID: qGrit, Value: "5"},
		{QuestionID: qGritRev2, Value: "2"},
		{QuestionID: qGritRev3, Value: "1"},
	}

	res := ComputeScores(answers, seedQuestionFixtures())

	if res.GritScore != 88 {
		t.Errorf("GritScore = %d, want 88", res.GritScore)
	}
	if got := res.TraitScores["GRIT"]; got != 88 {
		t.Errorf("TraitScores[GRIT] = %v, want 88", got)
	}
}

// TestComputeScores_ReverseScored proves a reverse-keyed Likert item flips its
// effective value: agreeing (5) with a reverse SN item must push AWAY from S.
func TestComputeScores_ReverseScored(t *testing.T) {
	questions := seedQuestionFixtures()

	agree := ComputeScores([]AnswerInput{{QuestionID: qSNRev, Value: "5"}}, questions)
	disagree := ComputeScores([]AnswerInput{{QuestionID: qSNRev, Value: "1"}}, questions)

	if got := agree.TraitScores["SN"]; got != 0 {
		t.Errorf("agree on reverse SN item: score = %v, want 0 (fully N)", got)
	}
	if got := disagree.TraitScores["SN"]; got != 100 {
		t.Errorf("disagree on reverse SN item: score = %v, want 100 (fully S)", got)
	}
}

// TestComputeScores_AttentionCheckIgnored proves the attention-check item
// never moves any score, whatever its answer.
func TestComputeScores_AttentionCheckIgnored(t *testing.T) {
	questions := seedQuestionFixtures()
	base := []AnswerInput{
		{QuestionID: qLikertEI, Value: "4"},
		{QuestionID: qGrit, Value: "5"},
	}
	withAttn := append(append([]AnswerInput{}, base...), AnswerInput{QuestionID: qAttn, Value: "1"})

	got := ComputeScores(withAttn, questions)
	want := ComputeScores(base, questions)

	if got.MBTIType != want.MBTIType || got.GritScore != want.GritScore {
		t.Errorf("attention check changed scores: got (%s, %d), want (%s, %d)",
			got.MBTIType, got.GritScore, want.MBTIType, want.GritScore)
	}
	for dim, v := range want.TraitScores {
		if got.TraitScores[dim] != v {
			t.Errorf("attention check changed %s: got %v, want %v", dim, got.TraitScores[dim], v)
		}
	}
}

// TestComputeScores_MBTIAlwaysValid asserts the type is always 4 letters, each
// from its dimension's own pole pair, across a spread of answer sets.
func TestComputeScores_MBTIAlwaysValid(t *testing.T) {
	questions := seedQuestionFixtures()
	poles := [4][2]byte{{'E', 'I'}, {'S', 'N'}, {'T', 'F'}, {'J', 'P'}}

	answerSets := [][]AnswerInput{
		nil, // no answers at all
		{{QuestionID: qLikertEI, Value: "1"}, {QuestionID: qLikertSN, Value: "5"}, {QuestionID: qLikertTF, Value: "1"}, {QuestionID: qLikertJP, Value: "5"}},
		{{QuestionID: qSJT1, Value: "A"}, {QuestionID: qSJT2, Value: "D"}, {QuestionID: qSJT3, Value: "C"}},
		{{QuestionID: qLikertEI, Value: "garbage"}, {QuestionID: qSJT1, Value: "Z"}},
	}

	for i, answers := range answerSets {
		res := ComputeScores(answers, questions)
		if len(res.MBTIType) != 4 {
			t.Fatalf("set %d: MBTIType %q is not 4 letters", i, res.MBTIType)
		}
		for d := 0; d < 4; d++ {
			if res.MBTIType[d] != poles[d][0] && res.MBTIType[d] != poles[d][1] {
				t.Errorf("set %d: letter %d = %c, want %c or %c", i, d, res.MBTIType[d], poles[d][0], poles[d][1])
			}
		}
	}
}

// TestComputeScores_GritBoundaries pins the scale ends: all-1 effective → 0,
// all-5 effective → 100.
func TestComputeScores_GritBoundaries(t *testing.T) {
	questions := seedQuestionFixtures()

	// qGrit is NOT reverse-scored; value maps straight to effective.
	low := ComputeScores([]AnswerInput{{QuestionID: qGrit, Value: "1"}}, questions)
	high := ComputeScores([]AnswerInput{{QuestionID: qGrit, Value: "5"}}, questions)

	if low.GritScore != 0 {
		t.Errorf("all-1 GritScore = %d, want 0", low.GritScore)
	}
	if high.GritScore != 100 {
		t.Errorf("all-5 GritScore = %d, want 100", high.GritScore)
	}
}

// TestComputeScores_EmptyDimensionFallsBack asserts an unanswered dimension
// lands on neutral 50 / first pole and is reported for Warn-logging.
func TestComputeScores_EmptyDimensionFallsBack(t *testing.T) {
	res := ComputeScores(nil, seedQuestionFixtures())

	if res.MBTIType != "ESTJ" {
		t.Errorf("MBTIType = %q, want ESTJ (all first poles)", res.MBTIType)
	}
	if res.GritScore != 50 {
		t.Errorf("GritScore = %d, want neutral 50", res.GritScore)
	}
	for _, dim := range []string{"EI", "SN", "TF", "JP", "GRIT"} {
		if got := res.TraitScores[dim]; got != 50 {
			t.Errorf("TraitScores[%s] = %v, want 50", dim, got)
		}
	}
	if len(res.NeutralFallbackDimensions) != 5 {
		t.Errorf("NeutralFallbackDimensions = %v, want all 5 dimensions", res.NeutralFallbackDimensions)
	}
}

// TestComputeScores_MalformedSJTAnswers asserts out-of-range option letters
// are skipped entirely while a mapped-but-neutral option ("E": {}) still
// counts toward the denominator, and nothing panics.
func TestComputeScores_MalformedSJTAnswers(t *testing.T) {
	questions := seedQuestionFixtures()

	// "Z" is not an option: the whole question must be a no-op.
	invalid := ComputeScores([]AnswerInput{
		{QuestionID: qLikertEI, Value: "4"},
		{QuestionID: qSJT1, Value: "Z"},
	}, questions)
	likertOnly := ComputeScores([]AnswerInput{{QuestionID: qLikertEI, Value: "4"}}, questions)
	if invalid.TraitScores["EI"] != likertOnly.TraitScores["EI"] {
		t.Errorf("invalid SJT option changed EI: got %v, want %v", invalid.TraitScores["EI"], likertOnly.TraitScores["EI"])
	}

	// "E" maps to {} — contributes 0 but widens EI's denominator:
	// sum=+1 (Likert), maxAbs=2+2 → round(50+1/4*50)=63 (vs 75 without SJT1).
	neutral := ComputeScores([]AnswerInput{
		{QuestionID: qLikertEI, Value: "4"},
		{QuestionID: qSJT1, Value: "E"},
	}, questions)
	if got := neutral.TraitScores["EI"]; got != 63 {
		t.Errorf("neutral SJT option: EI = %v, want 63", got)
	}
	if got := likertOnly.TraitScores["EI"]; got != 75 {
		t.Errorf("likert only: EI = %v, want 75", got)
	}
}
