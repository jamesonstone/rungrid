//go:build darwin || linux

package processcompose

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const directUnixSocketPathLimit = 90

func socketDialPath(socket string) (string, error) {
	if len(socket) <= directUnixSocketPathLimit {
		return socket, nil
	}
	directory := filepath.Join("/tmp", fmt.Sprintf("rungrid-uds-%d", os.Getuid()))
	if err := ensurePrivateAliasDirectory(directory); err != nil {
		return "", err
	}
	alias := filepath.Join(directory, socketAliasName(socket))
	if target, err := os.Readlink(alias); err == nil {
		if target != socket {
			return "", fmt.Errorf("process Compose socket alias target changed")
		}
		return alias, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect Process Compose socket alias: %w", err)
	}
	if err := os.Symlink(socket, alias); err != nil {
		if target, readErr := os.Readlink(alias); readErr == nil && target == socket {
			return alias, nil
		}
		return "", fmt.Errorf("create Process Compose socket alias: %w", err)
	}
	return alias, nil
}

func RemoveSocketAlias(socket string) error {
	if len(socket) <= directUnixSocketPathLimit {
		return nil
	}
	alias := filepath.Join("/tmp", fmt.Sprintf("rungrid-uds-%d", os.Getuid()), socketAliasName(socket))
	target, err := os.Readlink(alias)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || target != socket {
		return fmt.Errorf("refusing to remove a changed Process Compose socket alias")
	}
	if err := os.Remove(alias); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func ensurePrivateAliasDirectory(directory string) error {
	if err := os.Mkdir(directory, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("process Compose socket alias directory is not private")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) {
		return fmt.Errorf("process Compose socket alias directory ownership does not match")
	}
	return nil
}

func socketAliasName(socket string) string {
	hash := sha256.Sum256([]byte(socket))
	return hex.EncodeToString(hash[:16]) + ".sock"
}
