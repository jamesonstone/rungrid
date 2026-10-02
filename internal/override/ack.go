package override

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/jamesonstone/rungrid/internal/state"
)

// Exec acknowledgements prove which directory a service process really
// started in. `rungrid internal exec` writes one just before it replaces
// itself; applying an override reads it back. A runtime whose wrappers run an
// older Rungrid executable writes none, so an override it cannot honor is
// reported instead of silently claimed.
const ackDirectory = "exec-acks"

func ackRelative(generationID, service string) string {
	return filepath.Join(ackDirectory, generationID+"-"+service)
}

// WriteAck records the working directory a service is about to run in.
func WriteAck(layout state.Layout, generationID, service, workingDirectory string) error {
	return state.WriteFileAtomic(layout.ProjectDir, ackRelative(generationID, service), []byte(workingDirectory+"\n"), 0o600)
}

// ReadAck returns the recorded working directory, if any.
func ReadAck(layout state.Layout, generationID, service string) (string, bool) {
	content, err := os.ReadFile(filepath.Join(layout.ProjectDir, ackRelative(generationID, service)))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(content)), true
}

// RemoveAck forgets a service's acknowledgement before it restarts.
func RemoveAck(layout state.Layout, generationID, service string) {
	_ = os.Remove(filepath.Join(layout.ProjectDir, ackRelative(generationID, service)))
}
