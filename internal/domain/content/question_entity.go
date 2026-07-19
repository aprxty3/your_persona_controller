package content

// QuestionSection represents the assessment section a question belongs to.
type QuestionSection string

// The three assessment sections a question can belong to.
const (
	SectionA QuestionSection = "A"
	SectionB QuestionSection = "B"
	SectionC QuestionSection = "C"
)

// QuestionType represents the format of the question.
type QuestionType string

// The supported question formats.
const (
	TypeMultipleChoice QuestionType = "mc"
	TypeLikert         QuestionType = "likert"
	TypeEssayPrompt    QuestionType = "essay_prompt"
)

// Question is the locale-agnostic definition of an assessment question.
type Question struct {
	ID               string
	Section          QuestionSection
	Type             QuestionType
	IsReverseScored  bool
	IsAttentionCheck bool
	DisplayOrder     int
	Trait            string  // scoring dimension a Likert item measures (EI/SN/TF/JP/GRIT); empty when the item isn't scored numerically
	OptionTraitMap   *string // SJT only: JSON of per-option signed dimension points
}
