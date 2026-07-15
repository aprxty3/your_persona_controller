package assessment

import (
	"context"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

const (
	MascotStyleA    = "style_a"
	MascotStyleB    = "style_b"
	pdfSignedURLTTL = 15 * time.Minute
)

// ResultRepository is the narrow slice of TestResult persistence this usecase needs
type ResultRepository interface {
	FindByID(ctx context.Context, id string) (*testresult.TestResult, error)
	Update(ctx context.Context, result *testresult.TestResult) error
}

// PDFSignerService generates a time-limited download URL for a stored PDF object.
type PDFSignerService interface {
	PresignedGetURL(ctx context.Context, objectURL string, expiry time.Duration) (string, error)
}

// ResultResponse is the public shape of a TestResult returned to clients.
type ResultResponse struct {
	ResultID      string                 `json:"result_id"`
	MBTIType      string                 `json:"mbti_type"`
	GritScore     int                    `json:"grit_score"`
	TraitScores   map[string]interface{} `json:"trait_scores,omitempty"`
	AISummaryText string                 `json:"ai_summary_text"`
	Status        string                 `json:"status"`
	WellbeingFlag bool                   `json:"wellbeing_flag"`
	MascotStyle   string                 `json:"mascot_style"`
	Locale        string                 `json:"locale"`
	ShareToken    string                 `json:"share_token"`
	PDFStatus     string                 `json:"pdf_status"`
	IsOwner       bool                   `json:"is_owner"`
	CreatedAt     time.Time              `json:"created_at"`
}

// PDFStatusResponse reflects the async PDF generation lifecycle for polling clients.
type PDFStatusResponse struct {
	PDFStatus string `json:"pdf_status"`
}

// PDFDownloadResponse carries a short-lived signed URL to the generated PDF.
type PDFDownloadResponse struct {
	URL string `json:"url"`
}

// UpdateMascotStyleRequest carries the caller's identity for the ownership check
// alongside the new visual-only mascot variant.
type UpdateMascotStyleRequest struct {
	ResultID             string
	CallerUserID         string
	CallerGuestSessionID string
	MascotStyle          string
}

// ResultUseCase serves read/update access to a single TestResult
type ResultUseCase struct {
	repo   ResultRepository
	signer PDFSignerService
	log    logger.Logger
}

// NewResultUseCase constructs a ResultUseCase.
func NewResultUseCase(repo ResultRepository, signer PDFSignerService, log logger.Logger) *ResultUseCase {
	return &ResultUseCase{repo: repo, signer: signer, log: log.With("usecase", "result_query")}
}

// GetByID returns a TestResult's public detail view. Any caller holding the
// (unguessable UUID) result ID may view it.
func (uc *ResultUseCase) GetByID(ctx context.Context, id, callerUserID, callerGuestSessionID string) (*ResultResponse, error) {
	result, err := uc.findVisible(ctx, id)
	if err != nil {
		return nil, err
	}
	return toResultResponse(result, isResultOwner(result, callerUserID, callerGuestSessionID)), nil
}

// UpdateMascotStyle applies the caller's chosen visual variant — owner only.
func (uc *ResultUseCase) UpdateMascotStyle(ctx context.Context, req UpdateMascotStyleRequest) (*ResultResponse, error) {
	if req.MascotStyle != MascotStyleA && req.MascotStyle != MascotStyleB {
		return nil, fmt.Errorf("%w: mascot_style must be %q or %q", application.ErrInvalidInput, MascotStyleA, MascotStyleB)
	}

	result, err := uc.findOwned(ctx, req.ResultID, req.CallerUserID, req.CallerGuestSessionID)
	if err != nil {
		return nil, err
	}

	result.MascotStyle = req.MascotStyle
	if err := uc.repo.Update(ctx, result); err != nil {
		uc.log.Error("update mascot style failed", "step", "persist", "result_id", req.ResultID, "error", err)
		return nil, fmt.Errorf("update_mascot_style: %w", err)
	}

	return toResultResponse(result, true), nil
}

// GetPDFStatus reports the async PDF generation state for polling clients — owner only.
func (uc *ResultUseCase) GetPDFStatus(ctx context.Context, id, callerUserID, callerGuestSessionID string) (*PDFStatusResponse, error) {
	result, err := uc.findOwned(ctx, id, callerUserID, callerGuestSessionID)
	if err != nil {
		return nil, err
	}
	return &PDFStatusResponse{PDFStatus: string(result.PDFStatus)}, nil
}

// GetPDFDownloadURL mints a short-lived signed URL to the generated PDF — owner only.
func (uc *ResultUseCase) GetPDFDownloadURL(ctx context.Context, id, callerUserID, callerGuestSessionID string) (*PDFDownloadResponse, error) {
	result, err := uc.findOwned(ctx, id, callerUserID, callerGuestSessionID)
	if err != nil {
		return nil, err
	}
	if result.PDFUrl == nil || *result.PDFUrl == "" {
		return nil, application.ErrPDFNotReady
	}

	signedURL, err := uc.signer.PresignedGetURL(ctx, *result.PDFUrl, pdfSignedURLTTL)
	if err != nil {
		uc.log.Error("get pdf download url failed", "step", "sign", "result_id", id, "error", err)
		return nil, fmt.Errorf("get_pdf_download_url: %w", err)
	}
	return &PDFDownloadResponse{URL: signedURL}, nil
}

// findVisible loads a result and applies the retention visibility rule shared
// by every result-access path: a row past its expires_at is treated as 404
// whether or not the daily purge has physically removed it yet. This makes
// the purge design idempotent without an is_purging marker.
func (uc *ResultUseCase) findVisible(ctx context.Context, id string) (*testresult.TestResult, error) {
	result, err := uc.repo.FindByID(ctx, id)
	if err != nil {
		uc.log.Error("find result failed", "step", "lookup", "result_id", id, "error", err)
		return nil, fmt.Errorf("find_result: %w", err)
	}
	if result == nil || result.IsExpired() {
		return nil, application.ErrResultNotFound
	}
	return result, nil
}

// findOwned loads a visible result and enforces ownership
func (uc *ResultUseCase) findOwned(ctx context.Context, id, callerUserID, callerGuestSessionID string) (*testresult.TestResult, error) {
	result, err := uc.findVisible(ctx, id)
	if err != nil {
		return nil, err
	}
	if !isResultOwner(result, callerUserID, callerGuestSessionID) {
		return nil, application.ErrForbidden
	}
	return result, nil
}

// isResultOwner matches the caller's identity — Member access_token subject or
// Guest session_id cookie — against the result's recorded owner.
func isResultOwner(result *testresult.TestResult, callerUserID, callerGuestSessionID string) bool {
	if callerUserID != "" && result.UserID != nil && *result.UserID == callerUserID {
		return true
	}
	if callerGuestSessionID != "" && result.GuestSessionID != nil && *result.GuestSessionID == callerGuestSessionID {
		return true
	}
	return false
}

func toResultResponse(result *testresult.TestResult, isOwner bool) *ResultResponse {
	summary := ""
	if result.AISummaryText != nil {
		summary = *result.AISummaryText
	}
	return &ResultResponse{
		ResultID:      result.ID,
		MBTIType:      result.MBTIType,
		GritScore:     result.GritScore,
		TraitScores:   result.TraitScores,
		AISummaryText: summary,
		Status:        string(result.Status),
		WellbeingFlag: result.WellbeingFlag,
		MascotStyle:   result.MascotStyle,
		Locale:        result.Locale,
		ShareToken:    result.ShareToken,
		PDFStatus:     string(result.PDFStatus),
		IsOwner:       isOwner,
		CreatedAt:     result.CreatedAt,
	}
}
