package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/override/overridetest"
)

func runRoot(t *testing.T, fixture overridetest.Fixture, args ...string) (string, error) {
	t.Helper()
	root := newRootCommand()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stdout)
	root.SetIn(strings.NewReader(""))
	base := []string{"--config", filepath.Join(fixture.Workspace, ".rungrid.yaml"), "--state-dir", filepath.Join(fixture.Root, "state")}
	root.SetArgs(append(base, args...))
	err := root.Execute()
	return stdout.String(), err
}

func TestOverrideMutationsRequireAnActiveRuntime(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	for _, args := range [][]string{
		{"override", "set", "alpha", "GH-1"},
		{"override", "clear"},
		{"override", "clear", "svc"},
		{"override", "sync"},
		{"alpha", "GH-1"},
	} {
		_, err := runRoot(t, fixture, args...)
		if errs.Diagnostic(err) != "RG1815" || errs.Code(err) != errs.ExitConflict {
			t.Errorf("%v: %v (%s)", args, err, errs.Diagnostic(err))
		}
	}
	_, err := runRoot(t, fixture, "override", "set", "alpha", "GH-1")
	if !strings.Contains(err.Error(), "rungrid up --override alpha=GH-1") {
		t.Fatalf("missing up hint: %v", err)
	}
}

func TestOverrideListReportsAnEmptyEnvelopeWithoutARuntime(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	stdout, err := runRoot(t, fixture, "--json", "override", "list")
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		APIVersion string `json:"api_version"`
		Kind       string `json:"kind"`
		Data       struct {
			Overrides []any `json:"overrides"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(stdout), &envelope) != nil || envelope.APIVersion != "rungrid/output/v1" || envelope.Kind != "OverrideList" || envelope.Data.Overrides == nil {
		t.Fatalf("envelope = %s", stdout)
	}
	text, err := runRoot(t, fixture, "override", "list")
	if err != nil || !strings.Contains(text, "no overrides are active") {
		t.Fatalf("text = %q, %v", text, err)
	}
}

func TestServiceShortcutRefusals(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	for _, test := range []struct {
		args []string
		code string
	}{
		{[]string{"nope"}, "RG1820"},
		{[]string{"alpha", "GH-1", "extra"}, "RG1820"},
		{[]string{"alpha"}, "RG1819"},
		{[]string{"--json", "alpha"}, "RG1819"},
	} {
		if _, err := runRoot(t, fixture, test.args...); errs.Diagnostic(err) != test.code {
			t.Errorf("%v: %v, want %s", test.args, err, test.code)
		}
	}
}

func TestUpValidatesOverridesBeforeStarting(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	for _, test := range []struct {
		value, code string
	}{
		{"alpha", "RG1817"},
		{"nope=GH-1", "RG1805"},
		{"db=GH-1", "RG1806"},
		{"alpha=" + fixture.Foreign, "RG1810"},
	} {
		if _, err := runRoot(t, fixture, "up", "--no-open", "--override", test.value); errs.Diagnostic(err) != test.code {
			t.Errorf("--override %s: %v, want %s", test.value, err, test.code)
		}
	}
}

func TestUnknownCommandKeepsTypoSuggestions(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	_, err := runRoot(t, fixture, "stauts")
	if errs.Diagnostic(err) != "RG1820" || !strings.Contains(err.Error(), "Did you mean this?\n\tstatus") {
		t.Fatalf("err = %v", err)
	}
}
