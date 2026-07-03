package docker

import (
	"context"
	"testing"

	"stackyard/internal/storage"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
)

func TestBuildMySQLContainerSpec_RegularUser(t *testing.T) {
	svc := storage.Service{
		ID:                1,
		ProfileID:         2,
		Engine:            storage.EngineMySQL,
		ImageTag:          "mysql:8",
		HostPort:          13306,
		Username:          strPtr("appuser"),
		PasswordEncrypted: strPtr("s3cret"),
		DBName:            strPtr("appdb"),
		VolumeName:        "stackyard-vol-mysql-1",
	}

	cfg, hostCfg := buildMySQLContainerSpec(svc, "stackyard-profile-2")

	if cfg.Image != "mysql:8" {
		t.Errorf("Image = %q, want %q", cfg.Image, "mysql:8")
	}

	wantEnv := map[string]bool{
		"MYSQL_USER=appuser":         false,
		"MYSQL_PASSWORD=s3cret":      false,
		"MYSQL_ROOT_PASSWORD=s3cret": false,
		"MYSQL_DATABASE=appdb":       false,
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

	port := nat.Port("3306/tcp")
	if _, ok := cfg.ExposedPorts[port]; !ok {
		t.Errorf("ExposedPorts missing %s", port)
	}

	bindings, ok := hostCfg.PortBindings[port]
	if !ok || len(bindings) != 1 {
		t.Fatalf("PortBindings[%s] = %v, want exactly one binding", port, bindings)
	}
	if bindings[0].HostPort != "13306" {
		t.Errorf("HostPort = %q, want %q", bindings[0].HostPort, "13306")
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
	if m.Source != "stackyard-vol-mysql-1" {
		t.Errorf("Mount.Source = %q, want %q", m.Source, "stackyard-vol-mysql-1")
	}
	if m.Target != mysqlDataDir {
		t.Errorf("Mount.Target = %q, want %q", m.Target, mysqlDataDir)
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

func TestBuildMySQLContainerSpec_NilFieldsOmitted(t *testing.T) {
	svc := storage.Service{
		ID:         3,
		ProfileID:  4,
		Engine:     storage.EngineMySQL,
		ImageTag:   "mysql:8",
		HostPort:   13307,
		VolumeName: "stackyard-vol-mysql-3",
	}

	cfg, _ := buildMySQLContainerSpec(svc, "stackyard-profile-4")

	if len(cfg.Env) != 0 {
		t.Errorf("Env = %v, want empty when Username/Password/DBName are all nil", cfg.Env)
	}
}

func TestBuildMySQLContainerSpec_RootUsernameOmitsMySQLUser(t *testing.T) {
	svc := storage.Service{
		ID:                5,
		ProfileID:         6,
		Engine:            storage.EngineMySQL,
		ImageTag:          "mysql:8",
		HostPort:          13308,
		Username:          strPtr("root"),
		PasswordEncrypted: strPtr("rootpw"),
		DBName:            strPtr("appdb"),
		VolumeName:        "stackyard-vol-mysql-5",
	}

	cfg, _ := buildMySQLContainerSpec(svc, "stackyard-profile-6")

	wantEnv := map[string]bool{
		"MYSQL_ROOT_PASSWORD=rootpw": false,
		"MYSQL_DATABASE=appdb":       false,
	}
	if len(cfg.Env) != len(wantEnv) {
		t.Fatalf("Env = %v, want exactly %d entries (no MYSQL_USER/MYSQL_PASSWORD for root)", cfg.Env, len(wantEnv))
	}
	for _, e := range cfg.Env {
		if _, ok := wantEnv[e]; !ok {
			t.Errorf("unexpected env entry %q (root username must not set MYSQL_USER/MYSQL_PASSWORD)", e)
		}
		wantEnv[e] = true
	}
	for e, found := range wantEnv {
		if !found {
			t.Errorf("expected env entry %q not present in %v", e, cfg.Env)
		}
	}
}

func TestBuildMySQLContainerSpec_PasswordOnlyNoRegularUser(t *testing.T) {
	svc := storage.Service{
		ID:                7,
		ProfileID:         8,
		Engine:            storage.EngineMySQL,
		ImageTag:          "mysql:8",
		HostPort:          13309,
		PasswordEncrypted: strPtr("onlypw"),
		VolumeName:        "stackyard-vol-mysql-7",
	}

	cfg, _ := buildMySQLContainerSpec(svc, "stackyard-profile-8")

	if len(cfg.Env) != 1 {
		t.Fatalf("Env = %v, want exactly 1 entry", cfg.Env)
	}
	if cfg.Env[0] != "MYSQL_ROOT_PASSWORD=onlypw" {
		t.Errorf("Env[0] = %q, want %q", cfg.Env[0], "MYSQL_ROOT_PASSWORD=onlypw")
	}
}

func TestMySQLConnectionString_AllFieldsSet(t *testing.T) {
	svc := storage.Service{
		ID:                1,
		ProfileID:         2,
		Engine:            storage.EngineMySQL,
		ImageTag:          "mysql:8",
		HostPort:          3306,
		Username:          strPtr("appuser"),
		PasswordEncrypted: strPtr("s3cret"),
		DBName:            strPtr("appdb"),
		VolumeName:        "stackyard-vol-mysql-1",
	}

	got := MySQLConnectionString(svc)
	want := "mysql://appuser:s3cret@localhost:3306/appdb"
	if got != want {
		t.Errorf("MySQLConnectionString() = %q, want %q", got, want)
	}
}

func TestMySQLConnectionString_NilPassword(t *testing.T) {
	svc := storage.Service{
		ID:         2,
		ProfileID:  2,
		Engine:     storage.EngineMySQL,
		HostPort:   3307,
		Username:   strPtr("appuser"),
		DBName:     strPtr("appdb"),
		VolumeName: "stackyard-vol-mysql-2",
	}

	got := MySQLConnectionString(svc)
	want := "mysql://appuser@localhost:3307/appdb"
	if got != want {
		t.Errorf("MySQLConnectionString() = %q, want %q", got, want)
	}
}

func TestMySQLConnectionString_AllNilFallback(t *testing.T) {
	svc := storage.Service{
		ID:         3,
		ProfileID:  2,
		Engine:     storage.EngineMySQL,
		HostPort:   3308,
		VolumeName: "stackyard-vol-mysql-3",
	}

	got := MySQLConnectionString(svc)
	want := "mysql://root@localhost:3308/mysql"
	if got != want {
		t.Errorf("MySQLConnectionString() = %q, want %q", got, want)
	}
}

func TestMySQLConnectionString_SpecialCharactersEscaped(t *testing.T) {
	svc := storage.Service{
		ID:                4,
		ProfileID:         2,
		Engine:            storage.EngineMySQL,
		HostPort:          3309,
		Username:          strPtr("app user"),
		PasswordEncrypted: strPtr("p@ss:word/1"),
		DBName:            strPtr("appdb"),
		VolumeName:        "stackyard-vol-mysql-4",
	}

	got := MySQLConnectionString(svc)
	want := "mysql://app%20user:p%40ss%3Aword%2F1@localhost:3309/appdb"
	if got != want {
		t.Errorf("MySQLConnectionString() = %q, want %q", got, want)
	}
}

func TestStartMySQLEnvironment_WrongEngineRejected(t *testing.T) {
	svc := storage.Service{
		ID:         1,
		ProfileID:  1,
		Engine:     storage.EnginePostgres,
		HostPort:   3306,
		VolumeName: "stackyard-vol-mysql-wrong-engine",
	}

	c := &Client{}
	err := c.StartMySQLEnvironment(context.Background(), svc)
	if err == nil {
		t.Fatal("expected an error for a non-MySQL Service, got nil")
	}
}
