package docker

import (
	"math"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
)

const cpuPercentTolerance = 1e-9

func TestCPUPercentFromStats(t *testing.T) {
	tests := []struct {
		name     string
		cpuStats container.CPUStats
		preCPU   container.CPUStats
		wantPct  float64
	}{
		{
			name: "typical half-core-of-four load",
			cpuStats: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 2_000_000_000},
				SystemUsage: 100_000_000_000,
				OnlineCPUs:  4,
			},
			preCPU: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 1_000_000_000},
				SystemUsage: 90_000_000_000,
			},
			wantPct: 40.0,
		},
		{
			name: "zero cpu delta between reads",
			cpuStats: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 1_000_000_000},
				SystemUsage: 100_000_000_000,
				OnlineCPUs:  2,
			},
			preCPU: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 1_000_000_000},
				SystemUsage: 90_000_000_000,
			},
			wantPct: 0,
		},
		{
			name: "zero system delta avoids divide by zero",
			cpuStats: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 2_000_000_000},
				SystemUsage: 100_000_000_000,
				OnlineCPUs:  2,
			},
			preCPU: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 1_000_000_000},
				SystemUsage: 100_000_000_000,
			},
			wantPct: 0,
		},
		{
			name: "container just started, precpu all zero",
			cpuStats: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 0},
				SystemUsage: 0,
				OnlineCPUs:  2,
			},
			preCPU: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 0},
				SystemUsage: 0,
			},
			wantPct: 0,
		},
		{
			name: "counter reset produces negative delta, clamps to zero",
			cpuStats: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 500},
				SystemUsage: 100_000_000_000,
				OnlineCPUs:  2,
			},
			preCPU: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 1_000_000_000},
				SystemUsage: 90_000_000_000,
			},
			wantPct: 0,
		},
		{
			name: "online_cpus zero falls back to len(percpu_usage)",
			cpuStats: container.CPUStats{
				CPUUsage: container.CPUUsage{
					TotalUsage:  4_000_000_000,
					PercpuUsage: []uint64{1, 2, 3},
				},
				SystemUsage: 100_000_000_000,
				OnlineCPUs:  0,
			},
			preCPU: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 1_000_000_000},
				SystemUsage: 90_000_000_000,
			},
			wantPct: 90.0,
		},
		{
			name: "online_cpus and percpu_usage both empty falls back to one core",
			cpuStats: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 2_000_000_000},
				SystemUsage: 100_000_000_000,
				OnlineCPUs:  0,
			},
			preCPU: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 1_000_000_000},
				SystemUsage: 90_000_000_000,
			},
			wantPct: 10.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cpuPercentFromStats(tt.cpuStats, tt.preCPU)
			if math.Abs(got-tt.wantPct) > cpuPercentTolerance {
				t.Errorf("cpuPercentFromStats() = %v, want %v", got, tt.wantPct)
			}
		})
	}
}

func TestMemoryUsageBytesFromStats(t *testing.T) {
	tests := []struct {
		name string
		mem  container.MemoryStats
		want uint64
	}{
		{
			name: "no cache stats present, uses raw usage",
			mem:  container.MemoryStats{Usage: 1000},
			want: 1000,
		},
		{
			name: "cgroup v1 subtracts total_inactive_file",
			mem: container.MemoryStats{
				Usage: 1000,
				Stats: map[string]uint64{"total_inactive_file": 300},
			},
			want: 700,
		},
		{
			name: "cgroup v2 subtracts inactive_file",
			mem: container.MemoryStats{
				Usage: 1000,
				Stats: map[string]uint64{"inactive_file": 400},
			},
			want: 600,
		},
		{
			name: "cache value not smaller than usage is ignored",
			mem: container.MemoryStats{
				Usage: 1000,
				Stats: map[string]uint64{"total_inactive_file": 5000},
			},
			want: 1000,
		},
		{
			name: "zero usage, zero cache",
			mem:  container.MemoryStats{Usage: 0},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := memoryUsageBytesFromStats(tt.mem); got != tt.want {
				t.Errorf("memoryUsageBytesFromStats() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemoryPercentFromUsageAndLimit(t *testing.T) {
	tests := []struct {
		name         string
		usage, limit uint64
		want         float64
	}{
		{name: "typical half usage", usage: 512, limit: 1024, want: 50.0},
		{name: "zero limit avoids divide by zero", usage: 512, limit: 0, want: 0},
		{name: "zero usage", usage: 0, limit: 1024, want: 0},
		{name: "usage at limit", usage: 1024, limit: 1024, want: 100.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := memoryPercentFromUsageAndLimit(tt.usage, tt.limit); got != tt.want {
				t.Errorf("memoryPercentFromUsageAndLimit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainerResourceUsageFromStats(t *testing.T) {
	raw := container.StatsResponse{
		CPUStats: container.CPUStats{
			CPUUsage:    container.CPUUsage{TotalUsage: 2_000_000_000},
			SystemUsage: 100_000_000_000,
			OnlineCPUs:  4,
		},
		PreCPUStats: container.CPUStats{
			CPUUsage:    container.CPUUsage{TotalUsage: 1_000_000_000},
			SystemUsage: 90_000_000_000,
		},
		MemoryStats: container.MemoryStats{
			Usage: 1000,
			Limit: 2000,
			Stats: map[string]uint64{"total_inactive_file": 200},
		},
	}

	got := containerResourceUsageFromStats(raw)

	if math.Abs(got.CPUPercent-40.0) > cpuPercentTolerance {
		t.Errorf("CPUPercent = %v, want 40.0", got.CPUPercent)
	}
	if got.MemoryUsageBytes != 800 {
		t.Errorf("MemoryUsageBytes = %v, want 800", got.MemoryUsageBytes)
	}
	if got.MemoryLimitBytes != 2000 {
		t.Errorf("MemoryLimitBytes = %v, want 2000", got.MemoryLimitBytes)
	}
	if got.MemoryPercent != 40.0 {
		t.Errorf("MemoryPercent = %v, want 40.0", got.MemoryPercent)
	}
}

func TestWrapContainerStatsErr(t *testing.T) {
	err := wrapContainerStatsErr(sentinel)
	if err == nil || !strings.Contains(err.Error(), "get container stats") {
		t.Errorf("wrapContainerStatsErr message = %v", err)
	}
}

func TestWrapContainerStatsDecodeErr(t *testing.T) {
	err := wrapContainerStatsDecodeErr(sentinel)
	if err == nil || !strings.Contains(err.Error(), "decode container stats response") {
		t.Errorf("wrapContainerStatsDecodeErr message = %v", err)
	}
}
