// Package docker (this file, snapshot.go) assembles the real-time status
// dashboard payload (spec.md §3.5, tasks.md 2.8) from already-fetched
// profile/service data plus a batch of container states and
// ContainerStatsResult readings — see stats.go for how those readings are
// produced and lifecycle.go for ContainerStatesForNames.
//
// BuildEnvironmentStatusSnapshot itself is a pure function: it takes no
// Docker/DB dependency and only combines maps/slices already fetched by the
// caller, so it is unit-testable with fake states/stats data and no live
// Docker Engine (see snapshot_test.go). The caller responsible for actually
// fetching live data every poll cycle is App.buildStatusSnapshot (app.go),
// which also owns the background poller that emits this snapshot as the
// "environment:status" Wails event (task 2.8's push-update requirement).
package docker

import "stackyard/internal/storage"

// ProfileServices pairs a storage.Profile with the storage.Service rows that
// belong to it, so BuildEnvironmentStatusSnapshot doesn't need its own
// storage dependency to group services by profile.
type ProfileServices struct {
	Profile  storage.Profile
	Services []storage.Service
}

// EnvironmentStatusSnapshot is the full "environment:status" event payload:
// every profile, and within each, every one of its services' live state.
type EnvironmentStatusSnapshot struct {
	Profiles []ProfileStatus
}

// ProfileStatus is one profile's entry in EnvironmentStatusSnapshot.
type ProfileStatus struct {
	ProfileID   int64
	ProfileName string
	Services    []ServiceStatus
}

// ServiceStatus is one service's entry within a ProfileStatus, covering
// every field spec.md §3.5 requires the dashboard to show: service name,
// engine/version, state, mapped host port, CPU%, RAM usage.
//
// ServiceName is the service's Engine value (e.g. "postgres") — a profile
// has at most one service per engine (see app.go's duplicateEngineError), so
// the engine name alone already uniquely identifies a service within its
// profile; storage.Service carries no separate user-facing name field.
// EngineVersion is the service's ImageTag (e.g. "postgres:16-alpine") — the
// closest thing to a version string storage.Service carries.
//
// StatsAvailable is false whenever CPU/RAM couldn't be read this cycle
// (service not running, or the stats call itself errored) so the dashboard
// can distinguish "confirmed zero usage" from "no reading yet" rather than
// silently showing zeros for both.
type ServiceStatus struct {
	ServiceID        int64
	ServiceName      string
	Engine           string
	EngineVersion    string
	State            string
	HostPort         int
	CPUPercent       float64
	MemoryUsageBytes uint64
	MemoryLimitBytes uint64
	MemoryPercent    float64
	StatsAvailable   bool
}

// BuildEnvironmentStatusSnapshot assembles the dashboard payload from
// already-fetched profile/service data plus per-container state and stats
// readings, keyed by container name (see ServiceContainerName). A states or
// stats entry missing for a given service's container name is treated the
// same as an unreachable/never-started container rather than causing an
// error: State defaults to "unknown" and StatsAvailable stays false.
func BuildEnvironmentStatusSnapshot(profiles []ProfileServices, states map[string]string, stats map[string]ContainerStatsResult) EnvironmentStatusSnapshot {
	snapshot := EnvironmentStatusSnapshot{Profiles: make([]ProfileStatus, 0, len(profiles))}

	for _, p := range profiles {
		profileStatus := ProfileStatus{
			ProfileID:   p.Profile.ID,
			ProfileName: p.Profile.Name,
			Services:    make([]ServiceStatus, 0, len(p.Services)),
		}

		for _, svc := range p.Services {
			profileStatus.Services = append(profileStatus.Services, serviceStatusFor(svc, states, stats))
		}

		snapshot.Profiles = append(snapshot.Profiles, profileStatus)
	}

	return snapshot
}

func serviceStatusFor(svc storage.Service, states map[string]string, stats map[string]ContainerStatsResult) ServiceStatus {
	containerName := ServiceContainerName(svc.ID)

	state, ok := states[containerName]
	if !ok {
		state = "unknown"
	}

	status := ServiceStatus{
		ServiceID:     svc.ID,
		ServiceName:   string(svc.Engine),
		Engine:        string(svc.Engine),
		EngineVersion: svc.ImageTag,
		State:         state,
		HostPort:      svc.HostPort,
	}

	if result, ok := stats[containerName]; ok && result.Err == nil {
		status.CPUPercent = result.Usage.CPUPercent
		status.MemoryUsageBytes = result.Usage.MemoryUsageBytes
		status.MemoryLimitBytes = result.Usage.MemoryLimitBytes
		status.MemoryPercent = result.Usage.MemoryPercent
		status.StatsAvailable = true
	}

	return status
}
