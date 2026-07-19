// Package dto holds the HTTP request-body structs the handler layer binds
// and validates before mapping into application-layer use case requests.
package dto

// SubmitRequestDTO is the request body for POST /v1/assessment/submit.
type SubmitRequestDTO struct {
	Locale  string `json:"locale"`
	Answers []struct {
		QuestionID string `json:"question_id"`
		Value      string `json:"value"`
	} `json:"answers"`
}

// UpdateMascotStyleRequestDTO carries the caller's chosen visual mascot variant (FR-D11).
type UpdateMascotStyleRequestDTO struct {
	MascotStyle string `json:"mascot_style"`
}
