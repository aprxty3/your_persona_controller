// Package dto holds Data Transfer Objects for the pdf application layer that
// cross package boundaries — e.g. passed to the Maroto renderer
// implementation in internal/infrastructure/pdf. Kept separate from package
// pdf itself so that infrastructure implementations of pdf's interfaces (and
// their generated test mocks) reference this leaf package instead of
// importing pdf back, which would otherwise force pdf's own internal test
// files into an import cycle the moment they need a mock of PDFRenderer.
package dto

import "time"

// PDFData is everything the renderer needs to produce one report.
type PDFData struct {
	DisplayName         string
	Locale              string
	MBTIType            string
	GritScore           int
	TraitScores         map[string]interface{}
	AISummaryText       string
	EssayQuotes         []string
	StrengthsBlindSpots []string
	CreatedAt           time.Time
}
