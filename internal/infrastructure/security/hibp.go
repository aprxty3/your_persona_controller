package security

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application/auth"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

const hibpRangeURL = "https://api.pwnedpasswords.com/range/"

// HIBPBreachChecker implements auth.PasswordBreachChecker
type HIBPBreachChecker struct {
	httpClient *http.Client
	log        logger.Logger
}

// NewHIBPBreachChecker constructs a new HIBPBreachChecker.
func NewHIBPBreachChecker(log logger.Logger) auth.PasswordBreachChecker {
	return &HIBPBreachChecker{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		log:        log.With("service", "hibp"),
	}
}

// IsBreached reports whether password appears in the HIBP breach corpus.
func (c *HIBPBreachChecker) IsBreached(ctx context.Context, password string) (bool, error) {
	sum := sha1.Sum([]byte(password))
	hash := strings.ToUpper(hex.EncodeToString(sum[:]))
	prefix, suffix := hash[:5], hash[5:]

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, hibpRangeURL+prefix, nil)
	if err != nil {
		return false, fmt.Errorf("hibp: build request: %w", err)
	}
	req.Header.Set("Add-Padding", "true")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Warn("hibp check failed, failing open", "error", err)
		return false, fmt.Errorf("hibp: request range api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.log.Warn("hibp check failed, failing open", "status", resp.StatusCode)
		return false, fmt.Errorf("hibp: unexpected status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		suffixEntry, countStr, found := strings.Cut(scanner.Text(), ":")
		if !found || suffixEntry != suffix {
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(countStr))
		if err != nil {
			continue
		}
		return count > 0, nil
	}
	if err := scanner.Err(); err != nil {
		c.log.Warn("hibp check failed, failing open", "error", err)
		return false, fmt.Errorf("hibp: read range response: %w", err)
	}

	return false, nil
}
