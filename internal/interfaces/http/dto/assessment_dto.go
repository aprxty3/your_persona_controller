package dto

type SubmitRequestDTO struct {
	Locale  string `json:"locale"`
	Answers []struct {
		QuestionID string `json:"question_id"`
		Value      string `json:"value"`
	} `json:"answers"`
}
