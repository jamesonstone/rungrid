package present

import "strings"

// Dash marks an absent value in a table cell.
const Dash = "–"

// Section emoji. These label a block of output and are always emitted.
const (
	EmojiWorkspace = "🧭"
	EmojiServices  = "🧩"
	EmojiUp        = "🚀"
	EmojiDown      = "🛑"
	EmojiDoctor    = "🩺"
	EmojiPlan      = "🗺️"
	EmojiVersions  = "🧬"
	EmojiSync      = "🔄"
	EmojiWorktrees = "🌿"
	EmojiUninstall = "🧹"
	EmojiHooks     = "🪝"
	EmojiGenerate  = "📦"
	EmojiRepos     = "📚"
	EmojiArtifacts = "🗂️"
	EmojiRecovery  = "🛟"
	EmojiGuard     = "🛡️"
)

// Result glyphs.
const (
	GlyphOK      = "✅"
	GlyphWarning = "⚠️"
	GlyphError   = "❌"
	GlyphStep    = "▸"
	GlyphRunning = "🟢"
	GlyphPending = "🟡"
	GlyphFailed  = "🔴"
	GlyphIdle    = "⚪"
	GlyphExtern  = "🔌"
)

// Fallback renders an em-dash when a value is empty, so every table cell is
// occupied and columns stay readable.
func Fallback(value string) string {
	if strings.TrimSpace(value) == "" {
		return Dash
	}
	return value
}

// ServiceGlyph maps a Process Compose process status and health to a status
// glyph. The caller always renders the plain status word alongside it, so the
// glyph adds recognition speed and never carries meaning on its own.
func ServiceGlyph(status, health string) string {
	normalized := strings.ToLower(status)
	switch {
	case strings.Contains(strings.ToLower(health), "unhealthy"):
		return GlyphFailed
	case contains(normalized, "error", "failed", "terminated", "fatal"):
		return GlyphFailed
	// Exact match: "inactive" contains "active" as a substring, so the runtime
	// state must not be matched by containment.
	case normalized == "active":
		return GlyphRunning
	case contains(normalized, "running", "healthy"):
		return GlyphRunning
	// A degraded or stale runtime is a problem the reader must notice; it must
	// not fall through to the idle glyph.
	case contains(normalized, "degraded", "stale"):
		return GlyphWarning
	case contains(normalized, "launch", "pending", "waiting", "restart", "starting"):
		return GlyphPending
	case contains(normalized, "completed", "exited"):
		return GlyphOK
	case strings.Contains(normalized, "external"):
		return GlyphExtern
	default:
		return GlyphIdle
	}
}

// CheckGlyph maps a doctor check status to a result glyph.
func CheckGlyph(status string) string {
	switch strings.ToLower(status) {
	case "ok", "pass", "passed":
		return GlyphOK
	case "warning", "warn":
		return GlyphWarning
	case "error", "fail", "failed":
		return GlyphError
	default:
		return GlyphIdle
	}
}

// GuardGlyph maps a resource-guard health or per-service guard state to a
// status glyph. As everywhere else, the caller renders the plain state word
// beside it, so the glyph never carries the meaning by itself.
func GuardGlyph(state string) string {
	switch strings.ToLower(state) {
	case "healthy", "monitoring":
		return GlyphRunning
	case "starting", "learning":
		return GlyphPending
	case "breached", "circuit_open":
		return GlyphFailed
	case "degraded":
		return GlyphWarning
	case "observe_only":
		return GlyphExtern
	default:
		return GlyphIdle
	}
}

// ActionGlyph maps a repository-maintenance or reconciliation action to a
// result glyph.
func ActionGlyph(action string) string {
	normalized := strings.ToLower(action)
	switch {
	case strings.HasPrefix(normalized, "would-"):
		return GlyphStep
	case contains(normalized, "failed", "error", "blocked", "refused"):
		return GlyphError
	case contains(normalized, "preserved", "skipped", "none", "unchanged"):
		return GlyphIdle
	case normalized == "":
		return GlyphIdle
	default:
		return GlyphOK
	}
}

func contains(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
