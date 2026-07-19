package config

import (
	"os"

	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

// Check is one config value RequireProduction validates at boot.
type Check struct {
	Name            string
	Value           string
	InsecureDefault string
}

// RequireProduction fails fast — logs every violation in one line, then
// os.Exit(1) — if any check's Value is empty or still equals its
// InsecureDefault. Call this once at boot, gated by the caller on
// isProduction (APP_ENV=="production"); it does not read APP_ENV itself so
// callers don't parse the same env var twice.
func RequireProduction(log logger.Logger, checks ...Check) {
	var violations []string
	for _, c := range checks {
		if c.Value == "" || (c.InsecureDefault != "" && c.Value == c.InsecureDefault) {
			violations = append(violations, c.Name)
		}
	}
	if len(violations) > 0 {
		log.Error("refusing to start: APP_ENV=production but critical config is missing or still at its insecure default", "fields", violations)
		os.Exit(1)
	}
}
