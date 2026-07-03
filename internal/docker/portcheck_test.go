package docker

import "testing"

func TestEvaluateConflict_OwnRunningContainerIsNotAConflict(t *testing.T) {
	if got := evaluateConflict("running"); got != false {
		t.Errorf("evaluateConflict(%q) = %v, want false (own running container is not a conflict)", "running", got)
	}
}

func TestEvaluateConflict_AnyNonRunningStateIsAConflict(t *testing.T) {
	states := []string{"not_found", "exited", "created", "paused", "dead", "restarting", "removing", "unknown"}
	for _, state := range states {
		if got := evaluateConflict(state); got != true {
			t.Errorf("evaluateConflict(%q) = %v, want true", state, got)
		}
	}
}

func TestEvaluateConflict_MatchesCheckServicePortConflictContract(t *testing.T) {
	tests := []struct {
		name           string
		containerState string
		wantConflict   bool
	}{
		{name: "own container running -> no conflict", containerState: "running", wantConflict: false},
		{name: "own container never created -> conflict", containerState: "not_found", wantConflict: true},
		{name: "own container stopped -> conflict", containerState: "exited", wantConflict: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateConflict(tt.containerState)
			if got != tt.wantConflict {
				t.Errorf("evaluateConflict(%q) = %v, want %v", tt.containerState, got, tt.wantConflict)
			}
		})
	}
}
