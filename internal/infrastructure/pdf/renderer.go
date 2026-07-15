package pdf

import (
	"context"
	"fmt"
	"sort"

	"github.com/aprxty3/your_persona_controller.git/internal/application/pdf"
	"github.com/aprxty3/your_persona_controller.git/internal/application/pdf/dto"
	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/consts/pagesize"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

const gridSize = 12

var (
	brandColor = props.Color{Red: 79, Green: 70, Blue: 229}
	mutedColor = props.Color{Red: 148, Green: 163, Blue: 184}
	trackColor = props.Color{Red: 226, Green: 232, Blue: 240}
)

// MarotoRenderer is the maroto v2 implementation of pdf.PDFRenderer.
type MarotoRenderer struct{}

// placeholderSet is the locale-specific copy shown for sections whose source
// data doesn't exist yet (MBTIType/GritScore/TraitScores/StrengthsBlindSpots
// are zero-value until the scoring algorithm ticket lands).
type placeholderSet struct {
	MBTI      string
	Chart     string
	Insights  string
	Generated string
}

var placeholders = map[string]placeholderSet{
	"id": {
		MBTI:      "Belum tersedia — hasil akan muncul setelah pemrosesan skor selesai.",
		Chart:     "Grafik spektrum kepribadian & GRIT akan tersedia setelah skor kepribadian Anda diproses.",
		Insights:  "Ringkasan kekuatan & area pengembangan akan tersedia setelah skor kepribadian Anda diproses.",
		Generated: "Dibuat pada",
	},
	"en": {
		MBTI:      "Not yet available — your result will appear once scoring finishes processing.",
		Chart:     "The personality spectrum & GRIT chart will be available once your scores are processed.",
		Insights:  "Your strengths & growth areas summary will be available once your scores are processed.",
		Generated: "Generated on",
	},
}

func placeholderFor(locale string) placeholderSet {
	if p, ok := placeholders[locale]; ok {
		return p
	}
	return placeholders["en"]
}

// NewMarotoRenderer creates a new MarotoRenderer.
func NewMarotoRenderer() pdf.PDFRenderer {
	return &MarotoRenderer{}
}

func (r *MarotoRenderer) Render(_ context.Context, data dto.PDFData) ([]byte, error) {
	m := maroto.New(config.NewBuilder().WithPageSize(pagesize.A4).Build())
	ph := placeholderFor(data.Locale)

	addHeader(m, data, ph)
	addMBTISection(m, data, ph)
	addChartSection(m, data, ph)
	addInsightsSection(m, data, ph)
	addAIDeepDiveSection(m, data)
	addEssayQuotesSection(m, data)

	doc, err := m.Generate()
	if err != nil {
		return nil, fmt.Errorf("maroto: generate: %w", err)
	}
	return doc.GetBytes(), nil
}

func addHeader(m core.Maroto, data dto.PDFData, ph placeholderSet) {
	m.AddRow(16, text.NewCol(gridSize, "Your Persona's — Laporan Kepribadian", props.Text{
		Size: 18, Style: fontstyle.Bold, Align: align.Left,
	}))
	subtitle := fmt.Sprintf("%s | %s: %s", displayNameOrDefault(data.DisplayName), ph.Generated, data.CreatedAt.Format("2 January 2006"))
	m.AddRow(8, text.NewCol(gridSize, subtitle, props.Text{Size: 10, Color: &mutedColor}))
	m.AddRow(4, col.New(gridSize))
}

func displayNameOrDefault(name string) string {
	if name == "" {
		return "Anonim"
	}
	return name
}

func addMBTISection(m core.Maroto, data dto.PDFData, ph placeholderSet) {
	m.AddRow(8, text.NewCol(gridSize, "Tipe MBTI", props.Text{Size: 13, Style: fontstyle.Bold}))
	if data.MBTIType != "" {
		m.AddRow(10, text.NewCol(gridSize, data.MBTIType, props.Text{Size: 12, Style: fontstyle.Bold, Color: &brandColor}))
	} else {
		m.AddRow(10, text.NewCol(gridSize, ph.MBTI, props.Text{Size: 10, Style: fontstyle.Italic, Color: &mutedColor}))
	}
	m.AddRow(4, col.New(gridSize))
}

func addChartSection(m core.Maroto, data dto.PDFData, ph placeholderSet) {
	m.AddRow(8, text.NewCol(gridSize, "Spektrum Kepribadian & GRIT", props.Text{Size: 13, Style: fontstyle.Bold}))

	if len(data.TraitScores) == 0 && data.GritScore == 0 {
		m.AddRow(10, text.NewCol(gridSize, ph.Chart, props.Text{Size: 10, Style: fontstyle.Italic, Color: &mutedColor}))
		m.AddRow(4, col.New(gridSize))
		return
	}

	if data.GritScore > 0 {
		addBarRow(m, "GRIT", float64(data.GritScore), 100)
	}
	for _, trait := range sortedTraitKeys(data.TraitScores) {
		if trait == "GRIT" {
			continue // already drawn above from data.GritScore — same number, avoid a duplicate bar
		}
		v, ok := toFloat64(data.TraitScores[trait])
		if !ok {
			continue
		}
		addBarRow(m, trait, v, 100)
	}
	m.AddRow(4, col.New(gridSize))
}

// addBarRow draws one "label — proportional bar — value" row
func addBarRow(m core.Maroto, label string, value, max float64) {
	proportion := value / max
	if proportion < 0 {
		proportion = 0
	}
	if proportion > 1 {
		proportion = 1
	}
	filled := int(proportion * 9)
	empty := 9 - filled

	cols := []core.Col{
		text.NewCol(2, label, props.Text{Size: 9}),
	}
	if filled > 0 {
		cols = append(cols, col.New(filled).WithStyle(&props.Cell{BackgroundColor: &brandColor}))
	}
	if empty > 0 {
		cols = append(cols, col.New(empty).WithStyle(&props.Cell{BackgroundColor: &trackColor}))
	}
	cols = append(cols, text.NewCol(1, fmt.Sprintf("%.0f", value), props.Text{Size: 9, Align: align.Right}))

	m.AddRow(6, cols...)
}

func sortedTraitKeys(scores map[string]interface{}) []string {
	keys := make([]string, 0, len(scores))
	for k := range scores {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func addInsightsSection(m core.Maroto, data dto.PDFData, ph placeholderSet) {
	m.AddRow(8, text.NewCol(gridSize, "Kekuatan & Area Pengembangan", props.Text{Size: 13, Style: fontstyle.Bold}))
	if len(data.StrengthsBlindSpots) == 0 {
		m.AddRow(10, text.NewCol(gridSize, ph.Insights, props.Text{Size: 10, Style: fontstyle.Italic, Color: &mutedColor}))
	} else {
		for _, insight := range data.StrengthsBlindSpots {
			m.AddAutoRow(text.NewCol(gridSize, "• "+insight, props.Text{Size: 10}))
		}
	}
	m.AddRow(4, col.New(gridSize))
}

func addAIDeepDiveSection(m core.Maroto, data dto.PDFData) {
	m.AddRow(8, text.NewCol(gridSize, "AI Deep Dive", props.Text{Size: 13, Style: fontstyle.Bold}))
	if data.AISummaryText == "" {
		return
	}
	m.AddAutoRow(text.NewCol(gridSize, data.AISummaryText, props.Text{Size: 10}))
	m.AddRow(4, col.New(gridSize))
}

func addEssayQuotesSection(m core.Maroto, data dto.PDFData) {
	if len(data.EssayQuotes) == 0 {
		return
	}
	m.AddRow(8, text.NewCol(gridSize, "Kutipan Esai Anda", props.Text{Size: 13, Style: fontstyle.Bold}))
	for _, quote := range data.EssayQuotes {
		m.AddAutoRow(text.NewCol(gridSize, "\""+quote+"\"", props.Text{Size: 10, Style: fontstyle.Italic}))
	}
}
