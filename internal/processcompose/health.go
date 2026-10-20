package processcompose

import "strings"

// HealthReady reports whether a Process Compose readiness value is positive.
// Process Compose reports "Ready" and "Not Ready"; "healthy" is accepted for
// compatibility with older readiness values.
func HealthReady(health string) bool {
	normalized := strings.ToLower(strings.TrimSpace(health))
	if strings.Contains(normalized, "not ready") || strings.Contains(normalized, "unhealthy") {
		return false
	}
	return strings.Contains(normalized, "ready") || strings.Contains(normalized, "healthy")
}
