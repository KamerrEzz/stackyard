package dbengine

import (
	"testing"

	"stackyard/internal/storage"
)

func TestParseConnectionString_ValidCases(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want ConnectionFields
	}{
		{
			name: "postgres with password and database",
			raw:  "postgres://user:secret@localhost:5432/mydb",
			want: ConnectionFields{
				Engine:   storage.EnginePostgres,
				Host:     "localhost",
				Port:     5432,
				Username: "user",
				Password: "secret",
				Database: "mydb",
			},
		},
		{
			name: "mysql with password and database",
			raw:  "mysql://root:pw@127.0.0.1:3306/appdb",
			want: ConnectionFields{
				Engine:   storage.EngineMySQL,
				Host:     "127.0.0.1",
				Port:     3306,
				Username: "root",
				Password: "pw",
				Database: "appdb",
			},
		},
		{
			name: "mongodb with password and database",
			raw:  "mongodb://user:pass@host:27017/db",
			want: ConnectionFields{
				Engine:   storage.EngineMongoDB,
				Host:     "host",
				Port:     27017,
				Username: "user",
				Password: "pass",
				Database: "db",
			},
		},
		{
			name: "mongodb without database (optional)",
			raw:  "mongodb://user:pass@host:27017",
			want: ConnectionFields{
				Engine:   storage.EngineMongoDB,
				Host:     "host",
				Port:     27017,
				Username: "user",
				Password: "pass",
				Database: "",
			},
		},
		{
			name: "mongodb without password",
			raw:  "mongodb://user@host:27017/db",
			want: ConnectionFields{
				Engine:   storage.EngineMongoDB,
				Host:     "host",
				Port:     27017,
				Username: "user",
				Password: "",
				Database: "db",
			},
		},
		{
			name: "redis with password and db index",
			raw:  "redis://:secret@host:6379/0",
			want: ConnectionFields{
				Engine:   storage.EngineRedis,
				Host:     "host",
				Port:     6379,
				Username: "",
				Password: "secret",
				Database: "0",
			},
		},
		{
			name: "redis without password or db index",
			raw:  "redis://host:6379",
			want: ConnectionFields{
				Engine:   storage.EngineRedis,
				Host:     "host",
				Port:     6379,
				Username: "",
				Password: "",
				Database: "",
			},
		},
		{
			name: "postgres without password",
			raw:  "postgres://user@localhost:5432/mydb",
			want: ConnectionFields{
				Engine:   storage.EnginePostgres,
				Host:     "localhost",
				Port:     5432,
				Username: "user",
				Password: "",
				Database: "mydb",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConnectionString(tt.raw)
			if err != nil {
				t.Fatalf("ParseConnectionString(%q) returned unexpected error: %v", tt.raw, err)
			}

			if got.Engine != tt.want.Engine {
				t.Errorf("Engine = %q, want %q", got.Engine, tt.want.Engine)
			}
			if got.Host != tt.want.Host {
				t.Errorf("Host = %q, want %q", got.Host, tt.want.Host)
			}
			if got.Port != tt.want.Port {
				t.Errorf("Port = %d, want %d", got.Port, tt.want.Port)
			}
			if got.Username != tt.want.Username {
				t.Errorf("Username = %q, want %q", got.Username, tt.want.Username)
			}
			if got.Password != tt.want.Password {
				t.Errorf("Password = %q, want %q", got.Password, tt.want.Password)
			}
			if got.Database != tt.want.Database {
				t.Errorf("Database = %q, want %q", got.Database, tt.want.Database)
			}
		})
	}
}

func TestParseConnectionString_QueryParams(t *testing.T) {
	t.Run("postgres sslmode param", func(t *testing.T) {
		got, err := ParseConnectionString("postgres://user:pass@host:5432/db?sslmode=require")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Params.Get("sslmode") != "require" {
			t.Errorf("Params[sslmode] = %q, want %q", got.Params.Get("sslmode"), "require")
		}
	})

	t.Run("mongodb authSource param", func(t *testing.T) {
		got, err := ParseConnectionString("mongodb://user:pass@host:27017/db?authSource=admin")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Params.Get("authSource") != "admin" {
			t.Errorf("Params[authSource] = %q, want %q", got.Params.Get("authSource"), "admin")
		}
	})

	t.Run("multiple params", func(t *testing.T) {
		got, err := ParseConnectionString("postgres://user:pass@host:5432/db?sslmode=require&connect_timeout=10")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Params.Get("sslmode") != "require" || got.Params.Get("connect_timeout") != "10" {
			t.Errorf("Params = %v, want sslmode=require and connect_timeout=10", got.Params)
		}
	})
}

func TestParseConnectionString_MalformedCases(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{
			name:    "empty string",
			raw:     "",
			wantErr: "empty connection string",
		},
		{
			name:    "blank string",
			raw:     "   ",
			wantErr: "empty connection string",
		},
		{
			name:    "missing scheme separator",
			raw:     "localhost:5432/mydb",
			wantErr: `missing "://" scheme separator in connection string: expected one of postgres://, mysql://, mongodb://, redis://`,
		},
		{
			name:    "no scheme at all",
			raw:     "just some text",
			wantErr: `missing "://" scheme separator in connection string: expected one of postgres://, mysql://, mongodb://, redis://`,
		},
		{
			name:    "empty scheme before separator",
			raw:     "://host:5432/db",
			wantErr: "missing scheme in connection string: expected one of postgres, mysql, mongodb, redis",
		},
		{
			name:    "unsupported scheme",
			raw:     "ftp://host:21/db",
			wantErr: `unsupported scheme "ftp": expected one of postgres, mysql, mongodb, redis`,
		},
		{
			name:    "unsupported scheme sqlserver",
			raw:     "sqlserver://user:pass@host:1433/db",
			wantErr: `unsupported scheme "sqlserver": expected one of postgres, mysql, mongodb, redis`,
		},
		{
			name:    "missing host",
			raw:     "postgres:///db",
			wantErr: "missing host in connection string",
		},
		{
			name:    "missing host with userinfo",
			raw:     "postgres://user:pass@/db",
			wantErr: "missing host in connection string",
		},
		{
			name:    "non-numeric port",
			raw:     "postgres://user:pass@host:notaport/db",
			wantErr: `invalid port "notaport": not a valid port number`,
		},
		{
			name:    "negative port",
			raw:     "postgres://user:pass@host:-5/db",
			wantErr: `invalid port "-5": not a valid port number`,
		},
		{
			name:    "port out of range",
			raw:     "postgres://user:pass@host:99999/db",
			wantErr: "invalid port 99999: must be between 1 and 65535",
		},
		{
			name:    "port zero",
			raw:     "postgres://user:pass@host:0/db",
			wantErr: "invalid port 0: must be between 1 and 65535",
		},
		{
			name:    "trailing colon with no port",
			raw:     "postgres://user:pass@host:/db",
			wantErr: `missing port number after ":" in connection string`,
		},
		{
			name:    "malformed userinfo",
			raw:     "postgres://user:pa ss@host:5432/db",
			wantErr: "invalid userinfo in connection string: username/password contain characters that must be percent-encoded",
		},
		{
			name:    "redis with username",
			raw:     "redis://user:pass@host:6379/0",
			wantErr: `invalid userinfo "user": redis connection strings have no username, use redis://:password@host:port`,
		},
		{
			name:    "missing database for postgres",
			raw:     "postgres://user:pass@host:5432",
			wantErr: "missing database name in connection string: required for postgres",
		},
		{
			name:    "missing database for mysql",
			raw:     "mysql://user:pass@host:3306",
			wantErr: "missing database name in connection string: required for mysql",
		},
		{
			name:    "multi-segment database path",
			raw:     "postgres://user:pass@host:5432/db/extra",
			wantErr: `invalid database segment "/db/extra": expected a single path segment`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConnectionString(tt.raw)
			if err == nil {
				t.Fatalf("ParseConnectionString(%q) succeeded, want error %q", tt.raw, tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("ParseConnectionString(%q) error = %q, want %q", tt.raw, err.Error(), tt.wantErr)
			}
		})
	}
}
