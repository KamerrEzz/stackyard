package docker

import (
	"testing"

	"stackyard/internal/storage"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
)

func TestBuildMongoContainerSpec_AllFieldsSet(t *testing.T) {
	svc := storage.Service{
		ID:                10,
		ProfileID:         20,
		Engine:            storage.EngineMongoDB,
		ImageTag:          "mongo:7",
		HostPort:          27018,
		Username:          strPtr("appuser"),
		PasswordEncrypted: strPtr("s3cret"),
		DBName:            strPtr("appdb"),
		VolumeName:        "stackyard-vol-10",
	}

	cfg, hostCfg := buildMongoContainerSpec(svc, "stackyard-profile-20")

	if cfg.Image != "mongo:7" {
		t.Errorf("Image = %q, want %q", cfg.Image, "mongo:7")
	}

	wantEnv := map[string]bool{
		"MONGO_INITDB_ROOT_USERNAME=appuser": false,
		"MONGO_INITDB_ROOT_PASSWORD=s3cret":  false,
		"MONGO_INITDB_DATABASE=appdb":        false,
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

	port := nat.Port("27017/tcp")
	if _, ok := cfg.ExposedPorts[port]; !ok {
		t.Errorf("ExposedPorts missing %s", port)
	}

	bindings, ok := hostCfg.PortBindings[port]
	if !ok || len(bindings) != 1 {
		t.Fatalf("PortBindings[%s] = %v, want exactly one binding", port, bindings)
	}
	if bindings[0].HostPort != "27018" {
		t.Errorf("HostPort = %q, want %q", bindings[0].HostPort, "27018")
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
	if m.Source != "stackyard-vol-10" {
		t.Errorf("Mount.Source = %q, want %q", m.Source, "stackyard-vol-10")
	}
	if m.Target != mongoDataDir {
		t.Errorf("Mount.Target = %q, want %q", m.Target, mongoDataDir)
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

func TestBuildMongoContainerSpec_NilFieldsOmitted(t *testing.T) {
	svc := storage.Service{
		ID:         11,
		ProfileID:  21,
		Engine:     storage.EngineMongoDB,
		ImageTag:   "mongo:7",
		HostPort:   27019,
		VolumeName: "stackyard-vol-11",
	}

	cfg, _ := buildMongoContainerSpec(svc, "stackyard-profile-21")

	if len(cfg.Env) != 0 {
		t.Errorf("Env = %v, want empty when Username/Password/DBName are all nil", cfg.Env)
	}
}

func TestBuildMongoContainerSpec_EmptyDBNameOmitted(t *testing.T) {
	emptyDBName := ""
	svc := storage.Service{
		ID:         12,
		ProfileID:  22,
		Engine:     storage.EngineMongoDB,
		ImageTag:   "mongo:7",
		HostPort:   27020,
		Username:   strPtr("appuser"),
		DBName:     &emptyDBName,
		VolumeName: "stackyard-vol-12",
	}

	cfg, _ := buildMongoContainerSpec(svc, "stackyard-profile-22")

	for _, e := range cfg.Env {
		if e == "MONGO_INITDB_DATABASE=" {
			t.Errorf("Env = %v, want MONGO_INITDB_DATABASE omitted for an empty (not nil) DBName", cfg.Env)
		}
	}
}

func TestBuildMongoContainerSpec_PartialFields(t *testing.T) {
	svc := storage.Service{
		ID:         13,
		ProfileID:  23,
		Engine:     storage.EngineMongoDB,
		ImageTag:   "mongo:7",
		HostPort:   27021,
		Username:   strPtr("onlyuser"),
		VolumeName: "stackyard-vol-13",
	}

	cfg, _ := buildMongoContainerSpec(svc, "stackyard-profile-23")

	if len(cfg.Env) != 1 {
		t.Fatalf("Env = %v, want exactly 1 entry", cfg.Env)
	}
	if cfg.Env[0] != "MONGO_INITDB_ROOT_USERNAME=onlyuser" {
		t.Errorf("Env[0] = %q, want %q", cfg.Env[0], "MONGO_INITDB_ROOT_USERNAME=onlyuser")
	}
}

func TestMongoConnectionString_AllFieldsSet(t *testing.T) {
	svc := storage.Service{
		ID:                14,
		ProfileID:         24,
		Engine:            storage.EngineMongoDB,
		HostPort:          27017,
		Username:          strPtr("appuser"),
		PasswordEncrypted: strPtr("s3cret"),
		DBName:            strPtr("appdb"),
		VolumeName:        "stackyard-vol-14",
	}

	got := MongoConnectionString(svc)
	want := "mongodb://appuser:s3cret@localhost:27017/appdb"
	if got != want {
		t.Errorf("MongoConnectionString() = %q, want %q", got, want)
	}
}

func TestMongoConnectionString_NilPassword(t *testing.T) {
	svc := storage.Service{
		ID:         15,
		ProfileID:  24,
		Engine:     storage.EngineMongoDB,
		HostPort:   27022,
		Username:   strPtr("appuser"),
		DBName:     strPtr("appdb"),
		VolumeName: "stackyard-vol-15",
	}

	got := MongoConnectionString(svc)
	want := "mongodb://appuser@localhost:27022/appdb"
	if got != want {
		t.Errorf("MongoConnectionString() = %q, want %q", got, want)
	}
}

func TestMongoConnectionString_AllNilFallback(t *testing.T) {
	svc := storage.Service{
		ID:         16,
		ProfileID:  24,
		Engine:     storage.EngineMongoDB,
		HostPort:   27023,
		VolumeName: "stackyard-vol-16",
	}

	got := MongoConnectionString(svc)
	want := "mongodb://root@localhost:27023/admin"
	if got != want {
		t.Errorf("MongoConnectionString() = %q, want %q", got, want)
	}
}

func TestMongoConnectionString_SpecialCharactersEscaped(t *testing.T) {
	svc := storage.Service{
		ID:                17,
		ProfileID:         24,
		Engine:            storage.EngineMongoDB,
		HostPort:          27024,
		Username:          strPtr("app user"),
		PasswordEncrypted: strPtr("p@ss:word/1"),
		DBName:            strPtr("appdb"),
		VolumeName:        "stackyard-vol-17",
	}

	got := MongoConnectionString(svc)
	want := "mongodb://app%20user:p%40ss%3Aword%2F1@localhost:27024/appdb"
	if got != want {
		t.Errorf("MongoConnectionString() = %q, want %q", got, want)
	}
}
