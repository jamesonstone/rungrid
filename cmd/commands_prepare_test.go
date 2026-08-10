package cmd

import (
	"testing"
)

func TestUpCommandHasSyncFlag(t *testing.T) {
	opt := &options{}
	cmd := newUpCommand(opt)
	if cmd.Flags().Lookup("sync") == nil {
		t.Fatalf("expected 'sync' flag on up command")
	}
}
