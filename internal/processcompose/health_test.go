package processcompose

import "testing"

func TestHealthReady(t *testing.T) {
	for value, want := range map[string]bool{
		"Ready": true, "ready": true, "healthy": true,
		"Not Ready": false, "unhealthy": false, "": false, "-": false, "Unknown": false,
	} {
		if got := HealthReady(value); got != want {
			t.Errorf("HealthReady(%q) = %v, want %v", value, got, want)
		}
	}
}
