//go:build darwin || linux

package lifecycle

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/present"
)

func guardFixture() *guardstate.Status {
	return &guardstate.Status{
		Health:            "degraded",
		AuthorityValid:    false,
		HeartbeatAt:       "2026-08-18T10:00:00Z",
		DegradedReason:    "sampler stalled",
		GuardPID:          4211,
		GuardCPUPercent:   0.4,
		GuardRSSBytes:     12 * 1024 * 1024,
		SamplerDurationMS: 3.1,
		Scope: guardstate.AuthorityScope{
			GenerationID:            "generation-1",
			EffectiveManifestSHA256: "abcdef0123456789abcdef",
			RuntimePID:              4102,
			SocketPath:              "/tmp/rungrid.sock",
		},
		Services: []guardstate.ServiceStatus{{
			Name:            "api",
			State:           "breached",
			Enforcement:     "enforce",
			DegradedReason:  "cpu over limit",
			Metrics:         guardstate.Metrics{CPUPercent: 91.2, MemoryPercent: 40.5, Processes: 4, Threads: 22},
			EffectiveLimits: guardstate.Limits{CPUPercent: 80, MemoryPercent: 50, Processes: 16, Threads: 64},
			Baseline:        guardstate.Baseline{Mature: false, HealthyDuration: "4m"},
			RestartCount:    2,
			CircuitState:    "open",
			LatestIncident: &guardstate.IncidentSummary{
				OccurredAt: "2026-08-18T09:59:00Z", Tier: "hard", Trigger: "cpu", Action: "restart",
			},
		}},
	}
}

func TestGuardStatusColorlessEmitsNoANSI(t *testing.T) {
	t.Parallel()
	var buffer bytes.Buffer
	if err := writeStatusGuard(&buffer, present.New(false), guardFixture()); err != nil {
		t.Fatal(err)
	}
	output := buffer.String()
	if strings.Contains(output, "\033[") {
		t.Fatalf("colorless guard output contained ANSI escapes: %q", output)
	}
	// Every glyph must sit beside its plain word, and no limit may be dropped.
	for _, expected := range []string{
		present.EmojiGuard, "degraded", "sampler stalled", "12.0 MB", "generation-1", "abcdef012345",
		present.GuardGlyph("breached"), "breached", "enforce", "91.2% / 80.0%", "4 / 16", "22 / 64",
		"open", "baseline is still learning", "restarted 2 time(s)", "latest incident",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("guard output missing %q\n%s", expected, output)
		}
	}
}

func TestGuardStatusAbsentRendersNothing(t *testing.T) {
	t.Parallel()
	var buffer bytes.Buffer
	if err := writeStatusGuard(&buffer, present.New(false), nil); err != nil {
		t.Fatal(err)
	}
	if buffer.Len() != 0 {
		t.Fatalf("absent guard rendered output: %q", buffer.String())
	}
}

func TestGuardStatusReportsRuntimeVerification(t *testing.T) {
	t.Parallel()
	var buffer bytes.Buffer
	status := WorkspaceStatus{ProjectID: "demo", Runtime: "active", RuntimeVerification: "pid identity mismatch"}
	if err := WriteStatusHuman(&buffer, present.New(false), status); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buffer.String(), "runtime verification: pid identity mismatch") {
		t.Fatalf("status omitted the runtime verification warning:\n%s", buffer.String())
	}
}

func TestUnlimitedGuardDimensionsPrintObservationOnly(t *testing.T) {
	t.Parallel()
	if actual := ratioPercent(12.5, 0); actual != "12.5%" {
		t.Errorf("unlimited percent = %q", actual)
	}
	if actual := ratioCount(7, 0); actual != "7" {
		t.Errorf("unlimited count = %q", actual)
	}
	if actual := byteSize(900); actual != "900 B" {
		t.Errorf("byteSize(900) = %q", actual)
	}
}
