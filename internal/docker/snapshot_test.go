package docker

import (
	"errors"
	"testing"

	"stackyard/internal/storage"
)

func TestBuildEnvironmentStatusSnapshot_MixedRunningAndStopped(t *testing.T) {
	svcRunning := storage.Service{ID: 1, Engine: storage.EnginePostgres, ImageTag: "postgres:16-alpine", HostPort: 5432}
	svcStopped := storage.Service{ID: 2, Engine: storage.EngineRedis, ImageTag: "redis:7-alpine", HostPort: 6379}

	profiles := []ProfileServices{
		{
			Profile:  storage.Profile{ID: 10, Name: "dev"},
			Services: []storage.Service{svcRunning, svcStopped},
		},
	}

	states := map[string]string{
		ServiceContainerName(1): "running",
		ServiceContainerName(2): "exited",
	}
	stats := map[string]ContainerStatsResult{
		ServiceContainerName(1): {Usage: ContainerResourceUsage{CPUPercent: 2.5, MemoryUsageBytes: 1024, MemoryLimitBytes: 4096, MemoryPercent: 25}},
	}

	snapshot := BuildEnvironmentStatusSnapshot(profiles, states, stats)

	if len(snapshot.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(snapshot.Profiles))
	}
	got := snapshot.Profiles[0]
	if got.ProfileID != 10 || got.ProfileName != "dev" {
		t.Fatalf("unexpected profile identity: %+v", got)
	}
	if len(got.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(got.Services))
	}

	running := got.Services[0]
	if running.State != "running" || !running.StatsAvailable || running.CPUPercent != 2.5 || running.MemoryUsageBytes != 1024 {
		t.Errorf("unexpected running service status: %+v", running)
	}
	if running.ServiceName != "postgres" || running.Engine != "postgres" || running.EngineVersion != "postgres:16-alpine" {
		t.Errorf("unexpected running service identity fields: %+v", running)
	}

	stopped := got.Services[1]
	if stopped.State != "exited" || stopped.StatsAvailable {
		t.Errorf("unexpected stopped service status: %+v", stopped)
	}
	if stopped.CPUPercent != 0 || stopped.MemoryUsageBytes != 0 {
		t.Errorf("expected zero-value stats for a stopped service with no stats entry, got: %+v", stopped)
	}
}

func TestBuildEnvironmentStatusSnapshot_MissingStatsEntryForRunningService(t *testing.T) {
	svc := storage.Service{ID: 3, Engine: storage.EngineMySQL, ImageTag: "mysql:8", HostPort: 3306}
	profiles := []ProfileServices{{Profile: storage.Profile{ID: 20, Name: "solo"}, Services: []storage.Service{svc}}}

	states := map[string]string{ServiceContainerName(3): "running"}
	stats := map[string]ContainerStatsResult{}

	snapshot := BuildEnvironmentStatusSnapshot(profiles, states, stats)

	got := snapshot.Profiles[0].Services[0]
	if got.State != "running" {
		t.Errorf("State = %q, want %q", got.State, "running")
	}
	if got.StatsAvailable {
		t.Errorf("expected StatsAvailable = false when the container has no stats entry yet, got true")
	}
}

func TestBuildEnvironmentStatusSnapshot_StatsErrorMarksUnavailable(t *testing.T) {
	svc := storage.Service{ID: 4, Engine: storage.EngineMongoDB, ImageTag: "mongo:7", HostPort: 27017}
	profiles := []ProfileServices{{Profile: storage.Profile{ID: 21, Name: "solo-mongo"}, Services: []storage.Service{svc}}}

	states := map[string]string{ServiceContainerName(4): "running"}
	stats := map[string]ContainerStatsResult{
		ServiceContainerName(4): {Err: errors.New("container removed mid-poll")},
	}

	snapshot := BuildEnvironmentStatusSnapshot(profiles, states, stats)

	got := snapshot.Profiles[0].Services[0]
	if got.StatsAvailable {
		t.Errorf("expected StatsAvailable = false when the stats result carries an error, got true")
	}
	if got.CPUPercent != 0 || got.MemoryUsageBytes != 0 {
		t.Errorf("expected zero-value usage fields alongside StatsAvailable=false, got: %+v", got)
	}
}

func TestBuildEnvironmentStatusSnapshot_UnknownStateWhenMissingFromStatesMap(t *testing.T) {
	svc := storage.Service{ID: 5, Engine: storage.EngineRedis, ImageTag: "redis:7-alpine", HostPort: 6379}
	profiles := []ProfileServices{{Profile: storage.Profile{ID: 22, Name: "no-state"}, Services: []storage.Service{svc}}}

	snapshot := BuildEnvironmentStatusSnapshot(profiles, map[string]string{}, map[string]ContainerStatsResult{})

	got := snapshot.Profiles[0].Services[0]
	if got.State != "unknown" {
		t.Errorf("State = %q, want %q when the container name is absent from the states map", got.State, "unknown")
	}
}

func TestBuildEnvironmentStatusSnapshot_MultipleProfilesGroupedIndependently(t *testing.T) {
	svcA := storage.Service{ID: 6, Engine: storage.EnginePostgres, ImageTag: "postgres:16-alpine", HostPort: 5432}
	svcB := storage.Service{ID: 7, Engine: storage.EngineRedis, ImageTag: "redis:7-alpine", HostPort: 6379}

	profiles := []ProfileServices{
		{Profile: storage.Profile{ID: 30, Name: "profile-a"}, Services: []storage.Service{svcA}},
		{Profile: storage.Profile{ID: 31, Name: "profile-b"}, Services: []storage.Service{svcB}},
	}
	states := map[string]string{
		ServiceContainerName(6): "running",
		ServiceContainerName(7): "not_found",
	}

	snapshot := BuildEnvironmentStatusSnapshot(profiles, states, map[string]ContainerStatsResult{})

	if len(snapshot.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(snapshot.Profiles))
	}
	if snapshot.Profiles[0].ProfileID != 30 || snapshot.Profiles[1].ProfileID != 31 {
		t.Fatalf("expected profile order preserved as given, got: %+v", snapshot.Profiles)
	}
	if snapshot.Profiles[0].Services[0].State != "running" {
		t.Errorf("profile-a's service state = %q, want %q", snapshot.Profiles[0].Services[0].State, "running")
	}
	if snapshot.Profiles[1].Services[0].State != "not_found" {
		t.Errorf("profile-b's service state = %q, want %q", snapshot.Profiles[1].Services[0].State, "not_found")
	}
}

func TestBuildEnvironmentStatusSnapshot_EmptyProfilesProducesEmptySnapshot(t *testing.T) {
	snapshot := BuildEnvironmentStatusSnapshot(nil, nil, nil)
	if len(snapshot.Profiles) != 0 {
		t.Errorf("expected zero profiles, got %d", len(snapshot.Profiles))
	}
}
