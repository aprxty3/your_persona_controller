// Package i18n loads transactional-email message catalogs from external
// JSON files (locales/*.json) — separate from Go code so content/translator
// teams can edit copy without touching or recompiling business logic.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"

	pkglocale "github.com/aprxty3/your_persona_controller.git/pkg/locale"
)

//go:embed locales/*.json
var localeFiles embed.FS

// Message is a locale-specific subject/body pair for one message purpose
// (e.g. "otp_verification"). Body may contain a single %s placeholder
// (filled by the caller, e.g. with an OTP code) via fmt.Sprintf.
type Message struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// Catalog holds every transactional message, keyed [locale][purpose].
type Catalog struct {
	messages map[string]map[string]Message
}

// LoadCatalog parses every embedded locale file for the MVP-supported
// locales (pkg/locale.EN, pkg/locale.ID). It fails fast on malformed JSON —
// a broken catalog should stop the process from booting, not silently ship
// empty email bodies to production.
func LoadCatalog() (*Catalog, error) {
	messages := make(map[string]map[string]Message, 2)
	for _, loc := range []string{pkglocale.EN, pkglocale.ID} {
		data, err := localeFiles.ReadFile(fmt.Sprintf("locales/%s.json", loc))
		if err != nil {
			return nil, fmt.Errorf("i18n: read locale file %q: %w", loc, err)
		}
		var perPurpose map[string]Message
		if err := json.Unmarshal(data, &perPurpose); err != nil {
			return nil, fmt.Errorf("i18n: parse locale file %q: %w", loc, err)
		}
		messages[loc] = perPurpose
	}
	return &Catalog{messages: messages}, nil
}

// Message returns the subject/body pair for purpose in the requested
// locale, resolving unsupported/unset locales to EN via pkg/locale.Resolve —
// the same fallback authority used by QUESTION_TRANSLATION and
// INSIGHT_TEMPLATE lookups (FR-I9), so there is exactly one fallback rule
// in the codebase. ok is false when purpose doesn't exist in any locale.
func (c *Catalog) Message(purpose, locale string) (msg Message, ok bool) {
	perPurpose, found := c.messages[pkglocale.Resolve(locale)]
	if !found {
		return Message{}, false
	}
	msg, ok = perPurpose[purpose]
	return msg, ok
}
