package redis

import "strings"

// buildKey joins parts into a single colon-separated Redis key — the one
// place every namespaced key in this package (rate-limit counters, lock
// tokens, JTI denylist entries, ...) formats its key, instead of each file
// hand-rolling its own fmt.Sprintf("ns:%s:%s", ...) call.
func buildKey(parts ...string) string {
	return strings.Join(parts, ":")
}
