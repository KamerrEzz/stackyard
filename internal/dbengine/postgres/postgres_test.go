package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestNew_DoesNotDial(t *testing.T) {
	e := New("postgres://user:pass@nonexistent-host-for-test:5432/db")
	if e.pool != nil {
		t.Error("New must not create a pool before Connect is called")
	}
}

func TestPing_BeforeConnect_ReturnsErrNotConnected(t *testing.T) {
	e := New("postgres://user:pass@localhost:5432/db")
	if err := e.Ping(context.Background()); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Ping() before Connect = %v, want ErrNotConnected", err)
	}
}

func TestQuery_BeforeConnect_ReturnsErrNotConnected(t *testing.T) {
	e := New("postgres://user:pass@localhost:5432/db")
	if _, err := e.Query(context.Background(), "SELECT 1"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Query() before Connect = %v, want ErrNotConnected", err)
	}
}

func TestListSchemas_BeforeConnect_ReturnsErrNotConnected(t *testing.T) {
	e := New("postgres://user:pass@localhost:5432/db")
	if _, err := e.ListSchemas(context.Background()); !errors.Is(err, ErrNotConnected) {
		t.Errorf("ListSchemas() before Connect = %v, want ErrNotConnected", err)
	}
}

func TestListTables_BeforeConnect_ReturnsErrNotConnected(t *testing.T) {
	e := New("postgres://user:pass@localhost:5432/db")
	if _, err := e.ListTables(context.Background(), "public"); !errors.Is(err, ErrNotConnected) {
		t.Errorf("ListTables() before Connect = %v, want ErrNotConnected", err)
	}
}

func TestClose_BeforeConnect_IsSafe(t *testing.T) {
	e := New("postgres://user:pass@localhost:5432/db")
	if err := e.Close(); err != nil {
		t.Errorf("Close() before Connect = %v, want nil", err)
	}
	if err := e.Close(); err != nil {
		t.Errorf("second Close() = %v, want nil", err)
	}
}

func TestResolveTypeName_RegisteredOID_ReturnsPgtypeName(t *testing.T) {
	cases := []struct {
		name string
		oid  uint32
		want string
	}{
		{"int4", pgtype.Int4OID, "int4"},
		{"text", pgtype.TextOID, "text"},
		{"varchar", pgtype.VarcharOID, "varchar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveTypeName(tc.oid); got != tc.want {
				t.Errorf("resolveTypeName(%d) = %q, want %q", tc.oid, got, tc.want)
			}
		})
	}
}

func TestResolveTypeName_UnregisteredOID_FallsBackToOIDString(t *testing.T) {
	const unregisteredOID uint32 = 999999
	want := strconv.FormatUint(uint64(unregisteredOID), 10)
	if got := resolveTypeName(unregisteredOID); got != want {
		t.Errorf("resolveTypeName(%d) = %q, want fallback %q", unregisteredOID, got, want)
	}
}

func TestTranslatePgError_WrapsPgError(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "42601", Message: `syntax error at or near "SELCT"`}
	err := translatePgError("query", pgErr)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "42601") {
		t.Errorf("expected error to mention SQLSTATE code, got: %v", err)
	}
	if !strings.Contains(err.Error(), "syntax error") {
		t.Errorf("expected error to include Postgres's message, got: %v", err)
	}
	if !errors.Is(err, pgErr) {
		t.Error("expected translated error to wrap the original *pgconn.PgError")
	}
}

func TestTranslatePgError_PassesThroughNonPgError(t *testing.T) {
	generic := errors.New("connection reset by peer")
	err := translatePgError("query", generic)
	if !strings.Contains(err.Error(), "connection reset by peer") {
		t.Errorf("expected wrapped error to preserve original message, got: %v", err)
	}
	if !errors.Is(err, generic) {
		t.Error("expected translated error to wrap the original via %w")
	}
}
