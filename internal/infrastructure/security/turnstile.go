package security

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application/auth"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

const turnstileSiteVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// turnstileClient implements auth.TurnstileVerifier against the real
// Cloudflare Turnstile API.
type turnstileClient struct {
	httpClient *http.Client
	secretKey  string
	verifyURL  string
	log        logger.Logger
}

type turnstileSiteVerifyResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

// NewTurnstileVerifier returns a Turnstile verifier appropriate for the given
// secret key. An empty secretKey (local/dev environments without a Turnstile
// account) yields a no-op verifier that always passes — this is the ticket's
// explicit bypass rule, encapsulated here so callers never branch on "is dev
// mode" themselves.
func NewTurnstileVerifier(secretKey string, log logger.Logger) auth.TurnstileVerifier {
	if secretKey == "" {
		noopLog := log.With("service", "turnstile", "mode", "noop")
		noopLog.Warn("TURNSTILE_SECRET_KEY empty — bot verification bypassed, all tokens accepted")
		return &noopTurnstileVerifier{log: noopLog}
	}
	return &turnstileClient{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		secretKey:  secretKey,
		verifyURL:  turnstileSiteVerifyURL,
		log:        log.With("service", "turnstile"),
	}
}

// Verify calls Cloudflare's siteverify endpoint. On transport/timeout/status
// errors it fails open (returns true, err) — consistent with this codebase's
// HIBPBreachChecker precedent: a third-party outage degrades bot protection
// rather than blocking all registration/login/forgot-password traffic, since
// per-IP rate limiting and account lockout remain as the primary defenses.
// An explicit Cloudflare "not successful" verdict still fails closed (false, nil).
func (c *turnstileClient) Verify(ctx context.Context, token, remoteIP string) (bool, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	form := url.Values{
		"secret":   {c.secretKey},
		"response": {token},
	}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.verifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false, fmt.Errorf("turnstile: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Warn("turnstile verify failed, failing open", "error", err)
		return true, fmt.Errorf("turnstile: request siteverify api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.log.Warn("turnstile verify failed, failing open", "status", resp.StatusCode)
		return true, fmt.Errorf("turnstile: unexpected status %d", resp.StatusCode)
	}

	var result turnstileSiteVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.log.Warn("turnstile verify failed, failing open", "error", err)
		return true, fmt.Errorf("turnstile: decode siteverify response: %w", err)
	}

	if !result.Success {
		c.log.Warn("turnstile verification rejected", "error_codes", result.ErrorCodes)
	}
	return result.Success, nil
}

// noopTurnstileVerifier always passes — used when TURNSTILE_SECRET_KEY is
// unset so local development isn't blocked by a missing Cloudflare account.
type noopTurnstileVerifier struct {
	log logger.Logger
}

func (c *noopTurnstileVerifier) Verify(_ context.Context, _, _ string) (bool, error) {
	return true, nil
}
