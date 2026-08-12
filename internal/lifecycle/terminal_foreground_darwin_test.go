//go:build darwin

package lifecycle

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

const terminalForegroundHelper = "RUNGRID_TERMINAL_FOREGROUND_HELPER"

func TestPrepareTerminalForeground(t *testing.T) {
	if os.Getenv(terminalForegroundHelper) == "1" {
		assertForegroundChild(t)
		return
	}
	command := exec.Command(
		"/usr/bin/script", "-q", "/dev/null", os.Args[0],
		"-test.run=^TestPrepareTerminalForeground$",
	)
	command.Env = append(os.Environ(), terminalForegroundHelper+"=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("foreground helper failed: %v\n%s", err, output)
	}
}

func TestPrepareTerminalForegroundWithoutTTY(t *testing.T) {
	command := exec.Command("/usr/bin/true")
	command.Stdin = strings.NewReader("")
	command.Stdout = &bytes.Buffer{}
	command.Stderr = &bytes.Buffer{}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	restore, err := prepareTerminalForeground(command)
	if err != nil {
		t.Fatal(err)
	}
	if command.SysProcAttr.Foreground {
		t.Fatal("non-terminal command was marked as foreground")
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
}

func assertForegroundChild(t *testing.T) {
	command := exec.Command(
		"/bin/sh", "-c",
		`test "$(ps -o pgid= -p $$ | tr -d ' ')" = "$(ps -o tpgid= -p $$ | tr -d ' ')"`,
	)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	restore, err := prepareTerminalForeground(command)
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waitErr := command.Wait()
	restoreErr := restore()
	if waitErr != nil {
		t.Fatal(waitErr)
	}
	if restoreErr != nil {
		t.Fatal(restoreErr)
	}
}
