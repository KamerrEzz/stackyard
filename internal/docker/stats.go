// Package docker (this file, stats.go) polls per-container CPU/RAM usage for
// the real-time status view (spec.md §3.5).
//
// # ContainerStatsOneShot over the streaming ContainerStats
//
// docker/docker/client (pinned at v28.0.4+incompatible, see go.mod) exposes
// two ways to read the Engine API's "/containers/{id}/stats" endpoint:
// ContainerStats(ctx, id, stream bool) and ContainerStatsOneShot(ctx, id).
// With stream=false, ContainerStats still has the daemon "prime" one cgroup
// sample cycle before responding, adding latency per call. ContainerStatsOneShot
// is the SDK's dedicated single-snapshot call — the daemon does not wait to
// prime it — which fits a polling loop over N containers on a ≤2s refresh
// target (spec.md §3.5) far better than paying the prime delay per container
// per tick.
//
// # CPU% formula
//
// Docker's raw stats response gives cumulative counters, not an instantaneous
// percentage — the same two samples (cpu_stats and precpu_stats, i.e. "this
// read" and "the read before it") the `docker stats` CLI itself uses. CPU% is
// derived exactly the way the Docker CLI computes it:
//
//	cpuDelta    = cpu_stats.cpu_usage.total_usage - precpu_stats.cpu_usage.total_usage
//	systemDelta = cpu_stats.system_cpu_usage      - precpu_stats.system_cpu_usage
//	onlineCPUs  = cpu_stats.online_cpus, falling back to len(percpu_usage), falling back to 1
//	cpuPercent  = (cpuDelta / systemDelta) * onlineCPUs * 100
//
// A one-shot sample still returns two reads (Docker maintains the previous
// sample internally per container), so this formula applies even to a single
// ContainerStatsOneShot call, not just a streamed series.
//
// # Memory usage, cache-adjusted
//
// mem.Usage includes the kernel page cache attributed to the container's
// cgroup, which the Docker CLI subtracts out because page cache is
// reclaimable and not "used" memory in the way an operator cares about.
// memoryUsageBytesFromStats mirrors that: it subtracts total_inactive_file
// (cgroup v1) or inactive_file (cgroup v2) from mem.Usage when present.
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/docker/docker/api/types/container"
)

// ContainerResourceUsage is a single point-in-time CPU/RAM reading for one
// container, as shown by the real-time status view (spec.md §3.5).
type ContainerResourceUsage struct {
	CPUPercent       float64
	MemoryUsageBytes uint64
	MemoryLimitBytes uint64
	MemoryPercent    float64
}

// ContainerStatsResult pairs one container's ContainerResourceUsage with any
// error encountered while polling it, for use in a StatsForContainers batch
// where one container's failure must not hide the others' readings.
type ContainerStatsResult struct {
	Usage ContainerResourceUsage
	Err   error
}

// ContainerStats polls a single non-streaming CPU/RAM snapshot for the named
// container via the Engine API's stats endpoint (ContainerStatsOneShot; see
// the package doc comment above for why one-shot over streaming). Returns an
// error if the container doesn't exist or the engine call fails — callers
// that already track container existence (e.g. via ContainerState) should
// check that first if they want to distinguish "gone" from "stats call
// failed" in their own error handling.
func (c *Client) ContainerStats(ctx context.Context, containerName string) (ContainerResourceUsage, error) {
	reader, err := c.cli.ContainerStatsOneShot(ctx, containerName)
	if err != nil {
		return ContainerResourceUsage{}, wrapContainerStatsErr(err)
	}
	defer reader.Body.Close()

	var raw container.StatsResponse
	if err := json.NewDecoder(reader.Body).Decode(&raw); err != nil {
		return ContainerResourceUsage{}, wrapContainerStatsDecodeErr(err)
	}

	return containerResourceUsageFromStats(raw), nil
}

// StatsForContainers polls CPU/RAM for every name in containerNames
// concurrently, one Engine API call per container, and returns a result per
// name.
//
// Error handling: a failure polling one container (stopped mid-poll, removed
// outside the app, engine hiccup) is captured in that name's
// ContainerStatsResult.Err and does not affect any other container's result
// or cause StatsForContainers itself to return an error — a dashboard
// rendering "all profiles/services" (spec.md §3.5, task 2.8) needs the N-1
// containers that ARE still reachable even when one isn't.
func (c *Client) StatsForContainers(ctx context.Context, containerNames []string) map[string]ContainerStatsResult {
	results := make(map[string]ContainerStatsResult, len(containerNames))

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, name := range containerNames {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			usage, err := c.ContainerStats(ctx, name)

			mu.Lock()
			defer mu.Unlock()
			results[name] = ContainerStatsResult{Usage: usage, Err: err}
		}(name)
	}
	wg.Wait()

	return results
}

func containerResourceUsageFromStats(stats container.StatsResponse) ContainerResourceUsage {
	memoryUsageBytes := memoryUsageBytesFromStats(stats.MemoryStats)

	return ContainerResourceUsage{
		CPUPercent:       cpuPercentFromStats(stats.CPUStats, stats.PreCPUStats),
		MemoryUsageBytes: memoryUsageBytes,
		MemoryLimitBytes: stats.MemoryStats.Limit,
		MemoryPercent:    memoryPercentFromUsageAndLimit(memoryUsageBytes, stats.MemoryStats.Limit),
	}
}

func cpuPercentFromStats(cpuStats, preCPUStats container.CPUStats) float64 {
	cpuDelta := float64(cpuStats.CPUUsage.TotalUsage) - float64(preCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(cpuStats.SystemUsage) - float64(preCPUStats.SystemUsage)

	if cpuDelta <= 0 || systemDelta <= 0 {
		return 0
	}

	onlineCPUs := float64(cpuStats.OnlineCPUs)
	if onlineCPUs == 0 {
		onlineCPUs = float64(len(cpuStats.CPUUsage.PercpuUsage))
	}
	if onlineCPUs == 0 {
		onlineCPUs = 1
	}

	return (cpuDelta / systemDelta) * onlineCPUs * 100.0
}

func memoryUsageBytesFromStats(mem container.MemoryStats) uint64 {
	if inactive, ok := mem.Stats["total_inactive_file"]; ok && inactive < mem.Usage {
		return mem.Usage - inactive
	}
	if inactive, ok := mem.Stats["inactive_file"]; ok && inactive < mem.Usage {
		return mem.Usage - inactive
	}
	return mem.Usage
}

func memoryPercentFromUsageAndLimit(memoryUsageBytes, memoryLimitBytes uint64) float64 {
	if memoryLimitBytes == 0 {
		return 0
	}
	return float64(memoryUsageBytes) / float64(memoryLimitBytes) * 100.0
}

func wrapContainerStatsErr(err error) error {
	return fmt.Errorf("docker: get container stats: %w", err)
}

func wrapContainerStatsDecodeErr(err error) error {
	return fmt.Errorf("docker: decode container stats response: %w", err)
}
