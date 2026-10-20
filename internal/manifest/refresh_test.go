package manifest

import (
	"strings"
	"testing"
)

func TestValidateRefresh(t *testing.T) {
	valid := &Refresh{Restart: "dependencies", Watch: []string{"src/**/*.ts"}, Steps: []RefreshStep{
		{Name: "migrate", Run: Command{Argv: []string{"make", "migrate"}}, Watch: []string{"migrations"}},
	}}
	if problems := refreshProblems(valid); len(problems) != 0 {
		t.Fatalf("valid refresh rejected: %v", problems)
	}
	invalid := &Refresh{Restart: "sometimes", Dependencies: "maybe", Watch: []string{"../outside", "/abs"}, Steps: []RefreshStep{
		{Name: "migrate", Run: Command{Argv: []string{"make"}}},
		{Name: "migrate", Run: Command{}, Watch: []string{"a"}},
	}}
	joined := strings.Join(refreshProblems(invalid), "\n")
	for _, want := range []string{
		"refresh.restart", "refresh.dependencies", "refresh.watch[0]", "refresh.watch[1]",
		"refresh.steps[0].watch", "refresh.steps[1].name", "refresh.steps[1].run.argv",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %s in:\n%s", want, joined)
		}
	}
}

func refreshProblems(refresh *Refresh) []string {
	var problems []string
	validateRefresh(refresh, "services[0].refresh", func(field, message string) {
		problems = append(problems, field+": "+message)
	})
	return problems
}
