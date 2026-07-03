package docker

import (
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
)

func TestContainerStateFromInspect_Running(t *testing.T) {
	inspect := container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			State: &container.State{Running: true, Status: "running"},
		},
	}
	if got := containerStateFromInspect(inspect); got != "running" {
		t.Errorf("containerStateFromInspect() = %q, want %q", got, "running")
	}
}

func TestContainerStateFromInspect_NotRunningPassesThroughStatus(t *testing.T) {
	statuses := []string{"created", "exited", "paused", "restarting", "dead", "removing"}
	for _, status := range statuses {
		inspect := container.InspectResponse{
			ContainerJSONBase: &container.ContainerJSONBase{
				State: &container.State{Running: false, Status: status},
			},
		}
		if got := containerStateFromInspect(inspect); got != status {
			t.Errorf("status %q: containerStateFromInspect() = %q, want %q", status, got, status)
		}
	}
}

func TestContainerStateFromInspect_NilState(t *testing.T) {
	inspect := container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{State: nil},
	}
	if got := containerStateFromInspect(inspect); got != "unknown" {
		t.Errorf("containerStateFromInspect() = %q, want %q", got, "unknown")
	}
}

func TestWrapContainerStopErr(t *testing.T) {
	err := wrapContainerStopErr(sentinel)
	if err == nil || !strings.Contains(err.Error(), "stop container") {
		t.Errorf("wrapContainerStopErr message = %v", err)
	}
}
