package docker

import (
	"strings"
	"testing"

	"stackyard/internal/storage"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
)

func TestProfileNetworkName(t *testing.T) {
	got := ProfileNetworkName(42)
	want := "stackyard-profile-42"
	if got != want {
		t.Errorf("ProfileNetworkName(42) = %q, want %q", got, want)
	}
}

func TestServiceContainerName(t *testing.T) {
	got := ServiceContainerName(7)
	want := "stackyard-service-7"
	if got != want {
		t.Errorf("ServiceContainerName(7) = %q, want %q", got, want)
	}
}

func strPtr(s string) *string { return &s }

func TestBuildPostgresContainerSpec_AllFieldsSet(t *testing.T) {
	svc := storage.Service{
		ID:                1,
		ProfileID:         2,
		Engine:            storage.EnginePostgres,
		ImageTag:          "postgres:16-alpine",
		HostPort:          15432,
		Username:          strPtr("appuser"),
		PasswordEncrypted: strPtr("s3cret"),
		DBName:            strPtr("appdb"),
		VolumeName:        "stackyard-vol-1",
	}

	cfg, hostCfg := buildPostgresContainerSpec(svc, "stackyard-profile-2")

	if cfg.Image != "postgres:16-alpine" {
		t.Errorf("Image = %q, want %q", cfg.Image, "postgres:16-alpine")
	}

	wantEnv := map[string]bool{
		"POSTGRES_USER=appuser":    false,
		"POSTGRES_PASSWORD=s3cret": false,
		"POSTGRES_DB=appdb":        false,
	}
	if len(cfg.Env) != len(wantEnv) {
		t.Fatalf("Env = %v, want exactly %d entries", cfg.Env, len(wantEnv))
	}
	for _, e := range cfg.Env {
		if _, ok := wantEnv[e]; !ok {
			t.Errorf("unexpected env entry %q", e)
		}
		wantEnv[e] = true
	}
	for e, found := range wantEnv {
		if !found {
			t.Errorf("expected env entry %q not present in %v", e, cfg.Env)
		}
	}

	port := nat.Port("5432/tcp")
	if _, ok := cfg.ExposedPorts[port]; !ok {
		t.Errorf("ExposedPorts missing %s", port)
	}

	bindings, ok := hostCfg.PortBindings[port]
	if !ok || len(bindings) != 1 {
		t.Fatalf("PortBindings[%s] = %v, want exactly one binding", port, bindings)
	}
	if bindings[0].HostPort != "15432" {
		t.Errorf("HostPort = %q, want %q", bindings[0].HostPort, "15432")
	}
	if bindings[0].HostIP != "0.0.0.0" {
		t.Errorf("HostIP = %q, want %q", bindings[0].HostIP, "0.0.0.0")
	}

	if hostCfg.NetworkMode != container.NetworkMode("stackyard-profile-2") {
		t.Errorf("NetworkMode = %q, want %q", hostCfg.NetworkMode, "stackyard-profile-2")
	}

	if len(hostCfg.Mounts) != 1 {
		t.Fatalf("Mounts = %v, want exactly one mount", hostCfg.Mounts)
	}
	m := hostCfg.Mounts[0]
	if m.Type != mount.TypeVolume {
		t.Errorf("Mount.Type = %q, want %q", m.Type, mount.TypeVolume)
	}
	if m.Source != "stackyard-vol-1" {
		t.Errorf("Mount.Source = %q, want %q", m.Source, "stackyard-vol-1")
	}
	if m.Target != postgresDataDir {
		t.Errorf("Mount.Target = %q, want %q", m.Target, postgresDataDir)
	}

	if hostCfg.RestartPolicy.Name != container.RestartPolicyUnlessStopped {
		t.Errorf("RestartPolicy.Name = %q, want %q", hostCfg.RestartPolicy.Name, container.RestartPolicyUnlessStopped)
	}

	if cfg.Labels["stackyard.managed"] != "true" {
		t.Errorf("Labels[managed] = %q, want %q", cfg.Labels["stackyard.managed"], "true")
	}
	if cfg.Labels["stackyard.service_id"] != "1" {
		t.Errorf("Labels[service_id] = %q, want %q", cfg.Labels["stackyard.service_id"], "1")
	}
	if cfg.Labels["stackyard.profile_id"] != "2" {
		t.Errorf("Labels[profile_id] = %q, want %q", cfg.Labels["stackyard.profile_id"], "2")
	}
}

func TestBuildPostgresContainerSpec_NilFieldsOmitted(t *testing.T) {
	svc := storage.Service{
		ID:         3,
		ProfileID:  4,
		Engine:     storage.EnginePostgres,
		ImageTag:   "postgres:16-alpine",
		HostPort:   15433,
		VolumeName: "stackyard-vol-3",
	}

	cfg, _ := buildPostgresContainerSpec(svc, "stackyard-profile-4")

	if len(cfg.Env) != 0 {
		t.Errorf("Env = %v, want empty when Username/Password/DBName are all nil", cfg.Env)
	}
}

func TestBuildPostgresContainerSpec_PartialFields(t *testing.T) {
	svc := storage.Service{
		ID:         5,
		ProfileID:  6,
		Engine:     storage.EnginePostgres,
		ImageTag:   "postgres:16-alpine",
		HostPort:   15434,
		Username:   strPtr("onlyuser"),
		VolumeName: "stackyard-vol-5",
	}

	cfg, _ := buildPostgresContainerSpec(svc, "stackyard-profile-6")

	if len(cfg.Env) != 1 {
		t.Fatalf("Env = %v, want exactly 1 entry", cfg.Env)
	}
	if cfg.Env[0] != "POSTGRES_USER=onlyuser" {
		t.Errorf("Env[0] = %q, want %q", cfg.Env[0], "POSTGRES_USER=onlyuser")
	}
}

func TestWrapNetworkInspectErr(t *testing.T) {
	err := wrapNetworkInspectErr(sentinel)
	if err == nil || !strings.Contains(err.Error(), "inspect network") {
		t.Errorf("wrapNetworkInspectErr message = %v", err)
	}
}

func TestWrapContainerCreateErr(t *testing.T) {
	err := wrapContainerCreateErr(sentinel)
	if err == nil || !strings.Contains(err.Error(), "create container") {
		t.Errorf("wrapContainerCreateErr message = %v", err)
	}
}
