package docker

import "testing"

// TestEvaluateConflict_OwnRunningContainerIsNotAConflict is the trickiest
// part of task 1.5, tested explicitly per its instructions: a service
// starting/restarting while its OWN container is already running and
// legitimately bound to its own configured port must NOT be reported as a
// conflict — otherwise every restart of an already-running profile would
// falsely report one.
func TestEvaluateConflict_OwnRunningContainerIsNotAConflict(t *testing.T) {
	if got := evaluateConflict("running"); got != false {
		t.Errorf("evaluateConflict(%q) = %v, want false (own running container is not a conflict)", "running", got)
	}
}

// TestEvaluateConflict_AnyNonRunningStateIsAConflict covers every other
// simplified state ContainerState can report (see lifecycle.go): if this
// service's own container isn't the one holding the port, whatever else
// caused the OS-level bind to fail is a genuine conflict.
func TestEvaluateConflict_AnyNonRunningStateIsAConflict(t *testing.T) {
	states := []string{"not_found", "exited", "created", "paused", "dead", "restarting", "removing", "unknown"}
	for _, state := range states {
		if got := evaluateConflict(state); got != true {
			t.Errorf("evaluateConflict(%q) = %v, want true", state, got)
		}
	}
}

// TestEvaluateConflict_MatchesCheckServicePortConflictContract exercises
// the same branching CheckServicePortConflict performs (OS-level free vs.
// taken, then the own-container exemption) without needing a live Docker
// daemon, per task 1.5's instruction to test the exemption "without
// requiring a live Docker container just to prove the branching logic is
// correct."
//
// CheckServicePortConflict itself calls c.ContainerState directly (a method
// on *Client backed by the real Docker API client), which can't be stubbed
// without a live daemon — so these two tests instead exercise the exact
// same decision path via evaluateConflict, called with the state
// CheckServicePortConflict would have passed it, keeping the assertions
// tied to production logic without needing package-level indirection that
// the rest of this codebase doesn't otherwise use (see compose.go/
// lifecycle.go's own pattern of extracting pure decision functions for this
// same reason).
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
