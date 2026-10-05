package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jamesonstone/rungrid/internal/override"
	"github.com/spf13/cobra"
)

func sampleReport() override.Report {
	return override.Report{
		Operation: "set",
		Set: []override.Entry{{
			Repository: "svc", OriginalPath: "/ws/svc", Path: "/lanes/GH-1", Branch: "GH-1",
			HeadOID: "0123456789abcdef0123456789abcdef01234567", Dirty: true, Source: override.SourceCLI,
			Services:   []string{"alpha", "beta"},
			Reanchored: []override.Reanchor{{Service: "alpha", Field: "run.argv", Original: "../tools/run.sh", Resolved: "/ws/tools/run.sh"}},
		}},
		Cleared:  []override.Entry{},
		Services: []override.ServiceAction{{Name: "alpha", Repository: "svc", Activation: "workspace", Action: override.ActionRestarted}},
		Warnings: []string{"override checkout has uncommitted changes: /lanes/GH-1"},
	}
}

func TestOverrideReportTextAndJSON(t *testing.T) {
	t.Parallel()
	var text bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&text)
	if err := writeOverrideReport(command, &options{noColor: true}, "example-k7m4q2", sampleReport()); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Overrides set", "svc now runs from /lanes/GH-1 (GH-1)", "services: alpha, beta",
		"re-anchored alpha run.argv: ../tools/run.sh → /ws/tools/run.sh", "uncommitted changes", "restarted"} {
		if !strings.Contains(text.String(), expected) {
			t.Errorf("text output missing %q:\n%s", expected, text.String())
		}
	}
	var encoded bytes.Buffer
	command.SetOut(&encoded)
	if err := writeOverrideReport(command, &options{json: true}, "example-k7m4q2", sampleReport()); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Kind string `json:"kind"`
		Data struct {
			Set []struct {
				Repository string   `json:"repository"`
				Path       string   `json:"path"`
				Branch     string   `json:"branch"`
				HeadOID    string   `json:"head_oid"`
				Dirty      bool     `json:"dirty"`
				Services   []string `json:"services"`
			} `json:"set"`
			Services []map[string]string `json:"services"`
		} `json:"data"`
	}
	if err := json.Unmarshal(encoded.Bytes(), &envelope); err != nil || envelope.Kind != "OverrideReport" || len(envelope.Data.Set) != 1 {
		t.Fatalf("json = %s (%v)", encoded.String(), err)
	}
	entry := envelope.Data.Set[0]
	if entry.Repository != "svc" || entry.Path != "/lanes/GH-1" || entry.Branch != "GH-1" || len(entry.HeadOID) != 40 || !entry.Dirty || len(entry.Services) != 2 {
		t.Fatalf("entry = %#v", entry)
	}
}

func TestOverrideListText(t *testing.T) {
	t.Parallel()
	var text bytes.Buffer
	report := override.ListReport{Generation: "generation-a", Overrides: sampleReport().Set}
	if err := override.WriteListHuman(&text, presentStyle(&text, true), report); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Overrides (1)", "/lanes/GH-1", "GH-1", "0123456789ab", "yes", "alpha, beta"} {
		if !strings.Contains(text.String(), expected) {
			t.Errorf("list missing %q:\n%s", expected, text.String())
		}
	}
}

func TestPickerNavigatesWithArrowsAndVimKeys(t *testing.T) {
	t.Parallel()
	var model tea.Model = newPickerModel("title", "", []string{"a", "b", "c"})
	for _, key := range []tea.KeyMsg{{Type: tea.KeyDown}, {Type: tea.KeyRunes, Runes: []rune("j")}, {Type: tea.KeyRunes, Runes: []rune("j")}, {Type: tea.KeyRunes, Runes: []rune("k")}} {
		model, _ = model.Update(key)
	}
	model, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if picked := model.(pickerModel); picked.chosen != 1 || command == nil {
		t.Fatalf("chosen = %d", picked.chosen)
	}
	cancelled, _ := newPickerModel("title", "", []string{"a"}).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cancelled.(pickerModel).chosen != -1 {
		t.Fatal("esc did not cancel")
	}
	view := newPickerModel("title", "HEADER", []string{"a", "b"}).View()
	if !strings.Contains(view, "› ") || !strings.Contains(view, "HEADER") || !strings.Contains(view, "j/k") {
		t.Fatalf("view = %q", view)
	}
}

func TestWorktreeRowsShowRecencyStateAndCurrent(t *testing.T) {
	t.Parallel()
	updated := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	header, rows := worktreeRows([]override.Candidate{
		{Path: "/lanes/GH-1", Branch: "GH-1", HeadOID: "0123456789abcdef", Subject: "fix", Dirty: true, Current: true, UpdatedAt: updated, CreatedAt: updated},
		{Path: "/ws/svc", Branch: "main", Primary: true},
	})
	if !strings.Contains(header, "UPDATED") || !strings.Contains(header, "CREATED") || len(rows) != 2 {
		t.Fatalf("header %q rows %q", header, rows)
	}
	if !strings.HasPrefix(rows[0], "●") || !strings.Contains(rows[0], "dirty") || !strings.Contains(rows[0], "01234567") || !strings.Contains(rows[1], "main (primary)") {
		t.Fatalf("rows = %q", rows)
	}
}
