package locale

const (
	EN = "en"
	ID = "id"
)

// Resolve returns the requested locale if supported,
// otherwise it falls back to the default locale (EN).
func Resolve(requestedLocale string) string {
	switch requestedLocale {
	case EN, ID:
		return requestedLocale
	default:
		return EN
	}
}
