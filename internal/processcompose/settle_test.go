package processcompose

import (
	"context"
	"testing"
	"time"
)

type sequenceReader struct{ statuses []string }

func (r *sequenceReader) Get(context.Context, string) (ProcessState, error) {
	status := r.statuses[0]
	if len(r.statuses) > 1 {
		r.statuses = r.statuses[1:]
	}
	return ProcessState{Status: status}, nil
}

func TestSettled(t *testing.T) {
	for status, want := range map[string]bool{
		"Running": false, "Terminating": false, "Restarting": false, "Launching": false, "Pending": false,
		"Completed": true, "Stopped": true, "Disabled": true, "Skipped": true, "Error": true,
	} {
		if got := Settled(status); got != want {
			t.Errorf("Settled(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestWaitSettledWaitsOutTermination(t *testing.T) {
	reader := &sequenceReader{statuses: []string{"Terminating", "Terminating", "Completed"}}
	if err := WaitSettled(context.Background(), reader, "api", 2*time.Second); err != nil {
		t.Fatal(err)
	}
	stuck := &sequenceReader{statuses: []string{"Terminating"}}
	if err := WaitSettled(context.Background(), stuck, "api", 300*time.Millisecond); err == nil {
		t.Fatal("a process stuck terminating was reported settled")
	}
}
