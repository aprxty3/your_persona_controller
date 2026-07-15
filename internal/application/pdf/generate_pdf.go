package pdf

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/content"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

const pdfContentType = "application/pdf"

// PDFRenderer turns assembled report data into PDF bytes.
type PDFRenderer interface {
	Render(ctx context.Context, data PDFData) ([]byte, error)
}

// PDFUploader stores rendered PDF bytes and returns their public/stored URL.
type PDFUploader interface {
	Upload(ctx context.Context, key string, data []byte, contentType string) (url string, err error)
}

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

// GeneratePDFUseCase implements the generate:pdf worker job : render a report from a completed TestResult and upload it to
// R2/MinIO, tracking progress via TestResult.pdf_status.
type GeneratePDFUseCase struct {
	testResultRepo   testresult.TestResultRepository
	answerRepo       testresult.AnswerRepository
	questionRepo     content.QuestionRepository
	insightRepo      content.InsightTemplateRepository
	userRepo         account.UserRepository
	guestSessionRepo account.GuestSessionRepository
	renderer         PDFRenderer
	uploader         PDFUploader
	log              logger.Logger
}

// NewGeneratePDFUseCase creates a new GeneratePDFUseCase.
func NewGeneratePDFUseCase(
	testResultRepo testresult.TestResultRepository,
	answerRepo testresult.AnswerRepository,
	questionRepo content.QuestionRepository,
	insightRepo content.InsightTemplateRepository,
	userRepo account.UserRepository,
	guestSessionRepo account.GuestSessionRepository,
	renderer PDFRenderer,
	uploader PDFUploader,
	log logger.Logger,
) *GeneratePDFUseCase {
	return &GeneratePDFUseCase{
		testResultRepo:   testResultRepo,
		answerRepo:       answerRepo,
		questionRepo:     questionRepo,
		insightRepo:      insightRepo,
		userRepo:         userRepo,
		guestSessionRepo: guestSessionRepo,
		renderer:         renderer,
		uploader:         uploader,
		log:              log.With("usecase", "generate_pdf"),
	}
}

// Execute renders and uploads the PDF for one test result. Errors returned
// after pdf_status has been marked "processing" are left for the caller to
// return unwrapped to Asynq's retry mechanism — only the worker's
// asynq.Config.ErrorHandler (final-failure hook) transitions pdf_status to
// "failed", not this use case.
func (uc *GeneratePDFUseCase) Execute(ctx context.Context, testResultID string) error {
	result, err := uc.testResultRepo.FindByID(ctx, testResultID)
	if err != nil {
		return fmt.Errorf("generate_pdf: find test result: %w", err)
	}
	if result == nil {
		uc.log.Warn("generate pdf dropped", "reason", "test_result_not_found", "test_result_id", testResultID)
		return nil
	}

	if err := uc.testResultRepo.UpdatePDFStatus(ctx, result.ID, nil, testresult.PDFStatusProcessing); err != nil {
		return fmt.Errorf("generate_pdf: mark processing: %w", err)
	}

	data, err := uc.buildData(ctx, result)
	if err != nil {
		return fmt.Errorf("generate_pdf: build data: %w", err)
	}

	pdfBytes, err := uc.renderer.Render(ctx, data)
	if err != nil {
		return fmt.Errorf("generate_pdf: render: %w", err)
	}

	key := objectKey(result)
	url, err := uc.uploader.Upload(ctx, key, pdfBytes, pdfContentType)
	if err != nil {
		return fmt.Errorf("generate_pdf: upload: %w", err)
	}

	if err := uc.testResultRepo.UpdatePDFStatus(ctx, result.ID, &url, testresult.PDFStatusCompleted); err != nil {
		return fmt.Errorf("generate_pdf: mark completed: %w", err)
	}

	uc.log.Info("pdf generated", "test_result_id", result.ID, "url", url)
	return nil
}

func (uc *GeneratePDFUseCase) buildData(ctx context.Context, result *testresult.TestResult) (PDFData, error) {
	displayName, err := uc.resolveDisplayName(ctx, result)
	if err != nil {
		return PDFData{}, err
	}

	essayQuotes, err := uc.collectEssayQuotes(ctx, result.ID)
	if err != nil {
		return PDFData{}, err
	}

	strengthsBlindSpots, err := uc.collectInsights(ctx, result)
	if err != nil {
		return PDFData{}, err
	}

	summary := ""
	if result.AISummaryText != nil {
		summary = *result.AISummaryText
	}

	return PDFData{
		DisplayName:         displayName,
		Locale:              result.Locale,
		MBTIType:            result.MBTIType,
		GritScore:           result.GritScore,
		TraitScores:         result.TraitScores,
		AISummaryText:       summary,
		EssayQuotes:         essayQuotes,
		StrengthsBlindSpots: strengthsBlindSpots,
		CreatedAt:           result.CreatedAt,
	}, nil
}

func (uc *GeneratePDFUseCase) resolveDisplayName(ctx context.Context, result *testresult.TestResult) (string, error) {
	if result.UserID != nil {
		u, err := uc.userRepo.FindByID(ctx, *result.UserID)
		if err != nil {
			return "", fmt.Errorf("lookup user: %w", err)
		}
		if u != nil {
			return u.DisplayName, nil
		}
		return "", nil
	}
	if result.GuestSessionID != nil {
		g, err := uc.guestSessionRepo.FindBySessionID(ctx, *result.GuestSessionID)
		if err != nil {
			return "", fmt.Errorf("lookup guest session: %w", err)
		}
		if g != nil {
			return g.DisplayName, nil
		}
	}
	return "", nil
}

// collectEssayQuotes gathers the user's own essay answer text
func (uc *GeneratePDFUseCase) collectEssayQuotes(ctx context.Context, testResultID string) ([]string, error) {
	answers, err := uc.answerRepo.FindByTestResultID(ctx, testResultID)
	if err != nil {
		return nil, fmt.Errorf("find answers: %w", err)
	}
	if len(answers) == 0 {
		return nil, nil
	}

	questionIDs := make([]string, 0, len(answers))
	seen := make(map[string]struct{}, len(answers))
	for _, a := range answers {
		if _, ok := seen[a.QuestionID]; ok {
			continue
		}
		seen[a.QuestionID] = struct{}{}
		questionIDs = append(questionIDs, a.QuestionID)
	}

	questions, err := uc.questionRepo.FindByIDs(ctx, questionIDs)
	if err != nil {
		return nil, fmt.Errorf("find questions: %w", err)
	}
	questionByID := make(map[string]content.Question, len(questions))
	for _, q := range questions {
		questionByID[q.ID] = q
	}

	var quotes []string
	for _, a := range answers {
		if q, ok := questionByID[a.QuestionID]; ok && q.Type == content.TypeEssayPrompt {
			quotes = append(quotes, a.Value)
		}
	}
	return quotes, nil
}

// collectInsights matches TraitScores against the InsightTemplate registry
// for the "Strengths & Blind Spots" section. Only templates whose condition
// actually holds for this result are included: threshold templates require
// the trait value to reach ThresholdValue; increase/decrease templates need a
// previous result to diff against, which this single-result job doesn't have,
// so they're skipped (they belong to the dashboard trend surface).
func (uc *GeneratePDFUseCase) collectInsights(ctx context.Context, result *testresult.TestResult) ([]string, error) {
	var texts []string
	for trait, raw := range result.TraitScores {
		value, ok := toNumeric(raw)
		if !ok {
			continue
		}
		// The template registry stores traits lowercase ("grit"); trait_scores
		// keys are uppercase dimension codes ("GRIT") — normalize to match.
		templates, err := uc.insightRepo.FindMatchingTemplates(ctx, strings.ToLower(trait), result.Locale)
		if err != nil {
			return nil, fmt.Errorf("find matching templates for trait %q: %w", trait, err)
		}
		for _, t := range templates {
			if t.ConditionType != content.ConditionThreshold || t.ThresholdValue == nil || value < *t.ThresholdValue {
				continue
			}
			texts = append(texts, strings.ReplaceAll(t.TemplateText, "{value}", strconv.Itoa(int(value))))
		}
	}
	return texts, nil
}

// toNumeric coerces a jsonb-roundtripped trait score (float64 after
// unmarshal, int fresh from ComputeScores) into a comparable float64.
func toNumeric(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

// objectKey builds the mandatory R2 key convention from
// guest/{session_id}/{result_id}.pdf or member/{user_id}/{result_id}.pdf.
func objectKey(result *testresult.TestResult) string {
	if result.UserID != nil {
		return fmt.Sprintf("member/%s/%s.pdf", *result.UserID, result.ID)
	}
	return fmt.Sprintf("guest/%s/%s.pdf", *result.GuestSessionID, result.ID)
}
