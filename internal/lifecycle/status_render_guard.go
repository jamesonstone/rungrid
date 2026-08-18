//go:build darwin || linux

package lifecycle

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/present"
)

// writeStatusGuard renders the runtime resource guard: one authority summary,
// a per-service limits table, and incident notes. The guard is optional, so an
// absent guard renders nothing rather than an empty section.
func writeStatusGuard(w io.Writer, style present.Style, guard *guardstate.Status) error {
	if guard == nil {
		return nil
	}
	if err := writeGuardSummary(w, style, guard); err != nil {
		return err
	}
	table := style.NewTable("SERVICE", "GUARD", "ENFORCEMENT", "CPU", "MEMORY", "PROCESSES", "THREADS", "CIRCUIT")
	for _, service := range guard.Services {
		table.Row(
			present.GuardGlyph(service.State)+" "+service.Name,
			service.State,
			service.Enforcement,
			ratioPercent(service.Metrics.CPUPercent, service.EffectiveLimits.CPUPercent),
			ratioPercent(service.Metrics.MemoryPercent, service.EffectiveLimits.MemoryPercent),
			ratioCount(service.Metrics.Processes, service.EffectiveLimits.Processes),
			ratioCount(service.Metrics.Threads, service.EffectiveLimits.Threads),
			service.CircuitState,
		)
	}
	if err := table.Render(w, "the guard is not tracking any service yet"); err != nil {
		return err
	}
	if err := writeGuardServiceNotes(w, style, guard.Services); err != nil {
		return err
	}
	return present.Blank(w)
}

func writeGuardSummary(w io.Writer, style present.Style, guard *guardstate.Status) error {
	parts := []string{present.GuardGlyph(guard.Health) + " " + guard.Health}
	if !guard.AuthorityValid {
		parts = append(parts, style.Muted("scope-valid=false"))
	}
	if guard.HeartbeatAt != "" {
		parts = append(parts, style.Muted("heartbeat")+" "+guard.HeartbeatAt)
	}
	header := fmt.Sprintf("%s %s  %s\n", present.EmojiGuard, style.Bold("Resource guard"), strings.Join(parts, "  "))
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	if guard.DegradedReason != "" {
		if err := style.Note(w, present.GlyphWarning, "degraded: "+guard.DegradedReason); err != nil {
			return err
		}
	}
	overhead := fmt.Sprintf(
		"guard %s %s  %s %.1f%%  %s %s  %s %.1f ms",
		style.Muted("pid"), positiveNumber(guard.GuardPID),
		style.Muted("cpu"), guard.GuardCPUPercent,
		style.Muted("rss"), byteSize(guard.GuardRSSBytes),
		style.Muted("sampler"), guard.SamplerDurationMS,
	)
	if err := style.Note(w, present.GlyphStep, overhead); err != nil {
		return err
	}
	if err := writeGuardScope(w, style, guard.Scope); err != nil {
		return err
	}
	if incident := guard.LatestControlIncident; incident != nil {
		detail := fmt.Sprintf(
			"latest control incident %s  %s %s  %s %s",
			incident.OccurredAt,
			style.Muted("trigger"), incident.Trigger,
			style.Muted("action"), incident.Action,
		)
		if err := style.Note(w, present.GlyphWarning, detail); err != nil {
			return err
		}
	}
	return present.Blank(w)
}

func writeGuardScope(w io.Writer, style present.Style, scope guardstate.AuthorityScope) error {
	detail := fmt.Sprintf(
		"scope %s %s  %s %s  %s %s  %s %s",
		style.Muted("generation"), present.Fallback(scope.GenerationID),
		style.Muted("manifest"), present.Fallback(shortHash(scope.EffectiveManifestSHA256)),
		style.Muted("runtime-pid"), present.Fallback(positiveNumber(scope.RuntimePID)),
		style.Muted("socket"), present.Fallback(scope.SocketPath),
	)
	return style.Note(w, present.GlyphStep, detail)
}

// writeGuardServiceNotes reports only what the table cannot: an immature
// baseline, accumulated restarts, a degraded reason, and the latest incident.
// A service in steady state adds no lines.
func writeGuardServiceNotes(w io.Writer, style present.Style, services []guardstate.ServiceStatus) error {
	for _, service := range services {
		if service.DegradedReason != "" {
			if err := style.Note(w, present.GlyphWarning, service.Name+" degraded: "+service.DegradedReason); err != nil {
				return err
			}
		}
		if !service.Baseline.Mature {
			detail := service.Name + " baseline is still learning"
			if service.Baseline.HealthyDuration != "" {
				detail += " (" + service.Baseline.HealthyDuration + " healthy so far)"
			}
			if err := style.Note(w, present.GlyphPending, detail); err != nil {
				return err
			}
		}
		if service.RestartCount > 0 {
			detail := fmt.Sprintf("%s restarted %d time(s) by the guard", service.Name, service.RestartCount)
			if err := style.Note(w, present.GlyphWarning, detail); err != nil {
				return err
			}
		}
		if incident := service.LatestIncident; incident != nil {
			detail := fmt.Sprintf(
				"%s latest incident %s  %s %s  %s %s  %s %s",
				service.Name, incident.OccurredAt,
				style.Muted("tier"), incident.Tier,
				style.Muted("trigger"), incident.Trigger,
				style.Muted("action"), incident.Action,
			)
			if err := style.Note(w, present.GlyphWarning, detail); err != nil {
				return err
			}
		}
	}
	return nil
}

// ratioPercent renders an observed percentage against its effective limit. A
// zero limit means the dimension is unlimited, so only the observation prints.
func ratioPercent(observed, limit float64) string {
	if limit <= 0 {
		return fmt.Sprintf("%.1f%%", observed)
	}
	return fmt.Sprintf("%.1f%% / %.1f%%", observed, limit)
}

func ratioCount(observed, limit int) string {
	if limit <= 0 {
		return strconv.Itoa(observed)
	}
	return strconv.Itoa(observed) + " / " + strconv.Itoa(limit)
}

func byteSize(value uint64) string {
	const unit = 1024
	if value < unit {
		return strconv.FormatUint(value, 10) + " B"
	}
	size, exponent := float64(value)/unit, 0
	for size >= unit && exponent < 3 {
		size /= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %s", size, [...]string{"KB", "MB", "GB", "TB"}[exponent])
}

func shortHash(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
