package docker

import (
	"testing"

	"stackyard/internal/storage"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
)

func TestBuildRedisContainerSpec_WithPassword(t *testing.T) {
	svc := storage.Service{
		ID:                10,
		ProfileID:         20,
		Engine:            storage.EngineRedis,
		ImageTag:          "redis:7-alpine",
		HostPort:          16379,
		PasswordEncrypted: strPtr("s3cret"),
		VolumeName:        "stackyard-redis-vol-1",
	}

	cfg, hostCfg := buildRedisContainerSpec(svc, "stackyard-profile-20")

	if cfg.Image != "redis:7-alpine" {
		t.Errorf("Image = %q, want %q", cfg.Image, "redis:7-alpine")
	}

	wantCmd := []string{"redis-server", "--requirepass", "s3cret"}
	if len(cfg.Cmd) != len(wantCmd) {
		t.Fatalf("Cmd = %v, want %v", cfg.Cmd, wantCmd)
	}
	for i := range wantCmd {
		if cfg.Cmd[i] != wantCmd[i] {
			t.Errorf("Cmd[%d] = %q, want %q", i, cfg.Cmd[i], wantCmd[i])
		}
	}

	port := nat.Port("6379/tcp")
	if _, ok := cfg.ExposedPorts[port]; !ok {
		t.Errorf("ExposedPorts missing %s", port)
	}

	bindings, ok := hostCfg.PortBindings[port]
	if !ok || len(bindings) != 1 {
		t.Fatalf("PortBindings[%s] = %v, want exactly one binding", port, bindings)
	}
	if bindings[0].HostPort != "16379" {
		t.Errorf("HostPort = %q, want %q", bindings[0].HostPort, "16379")
	}
	if bindings[0].HostIP != "0.0.0.0" {
		t.Errorf("HostIP = %q, want %q", bindings[0].HostIP, "0.0.0.0")
	}

	if hostCfg.NetworkMode != container.NetworkMode("stackyard-profile-20") {
		t.Errorf("NetworkMode = %q, want %q", hostCfg.NetworkMode, "stackyard-profile-20")
	}

	if len(hostCfg.Mounts) != 1 {
		t.Fatalf("Mounts = %v, want exactly one mount", hostCfg.Mounts)
	}
	m := hostCfg.Mounts[0]
	if m.Type != mount.TypeVolume {
		t.Errorf("Mount.Type = %q, want %q", m.Type, mount.TypeVolume)
	}
	if m.Source != "stackyard-redis-vol-1" {
		t.Errorf("Mount.Source = %q, want %q", m.Source, "stackyard-redis-vol-1")
	}
	if m.Target != redisDataDir {
		t.Errorf("Mount.Target = %q, want %q", m.Target, redisDataDir)
	}

	if hostCfg.RestartPolicy.Name != container.RestartPolicyUnlessStopped {
		t.Errorf("RestartPolicy.Name = %q, want %q", hostCfg.RestartPolicy.Name, container.RestartPolicyUnlessStopped)
	}

	if cfg.Labels["stackyard.managed"] != "true" {
		t.Errorf("Labels[managed] = %q, want %q", cfg.Labels["stackyard.managed"], "true")
	}
	if cfg.Labels["stackyard.service_id"] != "10" {
		t.Errorf("Labels[service_id] = %q, want %q", cfg.Labels["stackyard.service_id"], "10")
	}
	if cfg.Labels["stackyard.profile_id"] != "20" {
		t.Errorf("Labels[profile_id] = %q, want %q", cfg.Labels["stackyard.profile_id"], "20")
	}
}

func TestBuildRedisContainerSpec_NoPassword_NoAuth(t *testing.T) {
	svc := storage.Service{
		ID:         11,
		ProfileID:  21,
		Engine:     storage.EngineRedis,
		ImageTag:   "redis:7-alpine",
		HostPort:   16380,
		VolumeName: "stackyard-redis-vol-2",
	}

	cfg, _ := buildRedisContainerSpec(svc, "stackyard-profile-21")

	if len(cfg.Cmd) != 0 {
		t.Errorf("Cmd = %v, want empty (image default, no auth) when PasswordEncrypted is nil", cfg.Cmd)
	}
}

func TestBuildRedisContainerSpec_UsernameAndDBNameIgnored(t *testing.T) {
	svc := storage.Service{
		ID:         12,
		ProfileID:  22,
		Engine:     storage.EngineRedis,
		ImageTag:   "redis:7-alpine",
		HostPort:   16381,
		Username:   strPtr("should-be-ignored"),
		DBName:     strPtr("should-be-ignored-too"),
		VolumeName: "stackyard-redis-vol-3",
	}

	cfg, _ := buildRedisContainerSpec(svc, "stackyard-profile-22")

	if len(cfg.Env) != 0 {
		t.Errorf("Env = %v, want empty — Redis has no username/dbname env vars", cfg.Env)
	}
	if len(cfg.Cmd) != 0 {
		t.Errorf("Cmd = %v, want empty — Username/DBName must not leak into Cmd", cfg.Cmd)
	}
}

func TestRedisConnectionString_WithPassword(t *testing.T) {
	svc := storage.Service{
		ID:                1,
		ProfileID:         2,
		Engine:            storage.EngineRedis,
		HostPort:          6379,
		PasswordEncrypted: strPtr("s3cret"),
		VolumeName:        "stackyard-redis-vol-4",
	}

	got := RedisConnectionString(svc)
	want := "redis://:s3cret@localhost:6379"
	if got != want {
		t.Errorf("RedisConnectionString() = %q, want %q", got, want)
	}
}

func TestRedisConnectionString_NoPassword(t *testing.T) {
	svc := storage.Service{
		ID:         2,
		ProfileID:  2,
		Engine:     storage.EngineRedis,
		HostPort:   6380,
		VolumeName: "stackyard-redis-vol-5",
	}

	got := RedisConnectionString(svc)
	want := "redis://localhost:6380"
	if got != want {
		t.Errorf("RedisConnectionString() = %q, want %q", got, want)
	}
}

func TestRedisConnectionString_UsernameAndDBNameIgnored(t *testing.T) {
	svc := storage.Service{
		ID:         3,
		ProfileID:  2,
		Engine:     storage.EngineRedis,
		HostPort:   6381,
		Username:   strPtr("should-be-ignored"),
		DBName:     strPtr("3"),
		VolumeName: "stackyard-redis-vol-6",
	}

	got := RedisConnectionString(svc)
	want := "redis://localhost:6381"
	if got != want {
		t.Errorf("RedisConnectionString() = %q, want %q (Username/DBName must not appear)", got, want)
	}
}

func TestRedisConnectionString_SpecialCharactersEscaped(t *testing.T) {
	svc := storage.Service{
		ID:                4,
		ProfileID:         2,
		Engine:            storage.EngineRedis,
		HostPort:          6382,
		PasswordEncrypted: strPtr("p@ss:word/1"),
		VolumeName:        "stackyard-redis-vol-7",
	}

	got := RedisConnectionString(svc)
	want := "redis://:p%40ss%3Aword%2F1@localhost:6382"
	if got != want {
		t.Errorf("RedisConnectionString() = %q, want %q", got, want)
	}
}
