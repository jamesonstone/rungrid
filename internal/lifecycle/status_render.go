//go:build darwin || linux

package lifecycle

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jamesonstone/rungrid/internal/present"
	"github.com/jamesonstone/rungrid/internal/workspace"
)

// WriteStatusHuman renders the workspace status for a human reader: one
// workspace summary line, the lifecycle block when a journal exists, the
// resource-guard block when a guard is running, and a box-drawn service table.
// Every glyph is paired with its plain status word so the output stays complete
// without color or emoji rendering.
func WriteStatusHuman(w io.Writer, style present.Style, status WorkspaceStatus) error {
	if err := writeStatusHeader(w, style, status); err != nil {
		return err
	}
	if noteworthyLifecycle(status.Lifecycle) {
		if err := writeStatusLifecycle(w, style, status.Lifecycle); err != nil {
			return err
		}
	}
	if err := writeStatusGuard(w, style, status.ResourceGuard); err != nil {
		return err
	}
	if err := style.HeaderCount(w, present.EmojiServices, "Services", len(status.Services)); err != nil {
		return err
	}
	table := style.NewTable("SERVICE", "STATUS", "HEALTH", "PID", "SOURCE", "ACTIVATION", "OWNER")
	for _, service := range status.Services {
		table.Row(
			present.ServiceGlyph(service.Status, service.Health)+" "+service.Name,
			serviceStatusText(service),
			service.Health,
			positiveNumber(service.PID),
			service.Source,
			service.Activation,
			serviceOwner(service),
		)
	}
	return table.Render(w, emptyServicesNote(status))
}

func emptyServicesNote(status WorkspaceStatus) string {
	if status.Runtime != "active" && status.Generation == "" {
		return "no runtime is active; run rungrid up to start this workspace"
	}
	return "no services are declared for this generation"
}

func writeStatusHeader(w io.Writer, style present.Style, status WorkspaceStatus) error {
	parts := []string{
		style.Bold(status.ProjectID),
		present.ServiceGlyph(status.Runtime, "") + " " + status.Runtime,
	}
	if status.Generation != "" {
		parts = append(parts, style.Muted("generation")+" "+status.Generation)
	}
	if status.PID != 0 {
		parts = append(parts, style.Muted("pid")+" "+strconv.Itoa(status.PID))
	}
	if _, err := fmt.Fprintf(w, "%s %s  %s\n", present.EmojiWorkspace, style.Bold("Workspace"), strings.Join(parts, "  ")); err != nil {
		return err
	}
	if status.RuntimeVerification != "" {
		if err := style.Note(w, present.GlyphWarning, "runtime verification: "+status.RuntimeVerification); err != nil {
			return err
		}
	}
	return present.Blank(w)
}

// noteworthyLifecycle suppresses the lifecycle block when it would only repeat
// what the workspace line already says. An unremarkable active journal with no
// hooks, no teardown obligation, and no failures carries no extra information.
func noteworthyLifecycle(lifecycleStatus *WorkspaceLifecycleStatus) bool {
	if lifecycleStatus == nil {
		return false
	}
	return lifecycleStatus.State != workspace.StateActive ||
		lifecycleStatus.TeardownRequired ||
		lifecycleStatus.CleanupFailure != "" ||
		lifecycleStatus.LastFailure != nil ||
		len(lifecycleStatus.CompletedBefore) > 0
}

func writeStatusLifecycle(w io.Writer, style present.Style, lifecycleStatus *WorkspaceLifecycleStatus) error {
	summary := fmt.Sprintf(
		"%s  %s  %s",
		lifecycleStatus.State,
		style.Muted(fmt.Sprintf("teardown-required=%t", lifecycleStatus.TeardownRequired)),
		style.Muted(fmt.Sprintf("completed-before-up=%d", len(lifecycleStatus.CompletedBefore))),
	)
	if _, err := fmt.Fprintf(w, "%s %s  %s\n", present.EmojiHooks, style.Bold("Lifecycle"), summary); err != nil {
		return err
	}
	if lifecycleStatus.CleanupFailure != "" {
		if err := style.Note(w, present.GlyphWarning, "cleanup failure: "+lifecycleStatus.CleanupFailure); err != nil {
			return err
		}
	}
	if failure := lifecycleStatus.LastFailure; failure != nil {
		detail := fmt.Sprintf("last failed command: %s/%s (%s)", failure.Phase, failure.Name, failure.Status)
		if err := style.Note(w, present.GlyphError, detail); err != nil {
			return err
		}
	}
	return present.Blank(w)
}

func serviceStatusText(service ServiceStatus) string {
	if service.ExitCode != 0 {
		return fmt.Sprintf("%s (exit %d)", service.Status, service.ExitCode)
	}
	return service.Status
}

func serviceOwner(service ServiceStatus) string {
	owners := []string{}
	if service.SessionOwned {
		owners = append(owners, "session")
	}
	if service.TabRegistered {
		owners = append(owners, "tab")
	}
	return strings.Join(owners, "+")
}

func positiveNumber(value int) string {
	if value <= 0 {
		return ""
	}
	return strconv.Itoa(value)
}
