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

// PickWithFallback picks an item based on locale preference
func PickWithFallback[T any](items []T, key func(T) string, itemLocale func(T) string, requested string) map[string]T {
	picked := make(map[string]T, len(items))
	matched := make(map[string]bool, len(items))
	for _, it := range items {
		if itemLocale(it) == requested {
			picked[key(it)] = it
			matched[key(it)] = true
		}
	}
	for _, it := range items {
		if itemLocale(it) == EN && !matched[key(it)] {
			picked[key(it)] = it
		}
	}
	return picked
}
