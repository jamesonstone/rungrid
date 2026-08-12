//go:build darwin || linux

package lifecycle

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

var terminalForegroundMu sync.Mutex

func prepareTerminalForeground(command *exec.Cmd) (func() error, error) {
	fd, foregroundPGID, found, err := commandTerminal(command)
	if err != nil {
		return nil, err
	}
	if !found {
		return func() error { return nil }, nil
	}
	parentPGID := unix.Getpgrp()
	if foregroundPGID != parentPGID {
		return nil, fmt.Errorf("rungrid process group %d does not own the terminal foreground", parentPGID)
	}
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	if command.SysProcAttr.Pgid != 0 {
		return nil, fmt.Errorf("interactive Process Compose client has a preassigned process group")
	}
	command.SysProcAttr.Setpgid = true
	command.SysProcAttr.Foreground = true
	command.SysProcAttr.Ctty = fd

	var restoreErr error
	var restoreOnce sync.Once
	return func() error {
		restoreOnce.Do(func() {
			restoreErr = setTerminalForeground(fd, foregroundPGID)
		})
		return restoreErr
	}, nil
}

func commandTerminal(command *exec.Cmd) (int, int, bool, error) {
	files := []*os.File{}
	if file, ok := command.Stdin.(*os.File); ok {
		files = append(files, file)
	}
	if file, ok := command.Stdout.(*os.File); ok {
		files = append(files, file)
	}
	if file, ok := command.Stderr.(*os.File); ok {
		files = append(files, file)
	}
	for _, file := range files {
		foregroundPGID, err := unix.IoctlGetInt(int(file.Fd()), unix.TIOCGPGRP)
		if err == nil {
			return int(file.Fd()), foregroundPGID, true, nil
		}
		if err != unix.ENOTTY {
			return 0, 0, false, fmt.Errorf("inspect terminal foreground: %w", err)
		}
	}
	return 0, 0, false, nil
}

func setTerminalForeground(fd, pgid int) error {
	terminalForegroundMu.Lock()
	defer terminalForegroundMu.Unlock()
	wasIgnored := signal.Ignored(syscall.SIGTTOU)
	signal.Ignore(syscall.SIGTTOU)
	err := unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, pgid)
	if !wasIgnored {
		signal.Reset(syscall.SIGTTOU)
	}
	if err != nil {
		return fmt.Errorf("restore terminal foreground: %w", err)
	}
	return nil
}
