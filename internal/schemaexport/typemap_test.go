package schemaexport

import (
	"testing"

	"stackyard/internal/dbengine"
)

func TestPrismaScalarFor_Postgres(t *testing.T) {
	for dataType, want := range postgresToPrismaScalar {
		if got := prismaScalarFor(dbengine.DialectPostgres, dataType); got != want {
			t.Errorf("prismaScalarFor(postgres, %q) = %q, want %q", dataType, got, want)
		}
	}
}

func TestPrismaScalarFor_MySQL(t *testing.T) {
	for dataType, want := range mysqlToPrismaScalar {
		if got := prismaScalarFor(dbengine.DialectMySQL, dataType); got != want {
			t.Errorf("prismaScalarFor(mysql, %q) = %q, want %q", dataType, got, want)
		}
	}
}

func TestPrismaScalarFor_UnknownTypeFallsBackToString(t *testing.T) {
	if got := prismaScalarFor(dbengine.DialectPostgres, "tsvector"); got != "String" {
		t.Errorf("prismaScalarFor(postgres, tsvector) = %q, want String", got)
	}
	if got := prismaScalarFor(dbengine.DialectMySQL, "geometry"); got != "String" {
		t.Errorf("prismaScalarFor(mysql, geometry) = %q, want String", got)
	}
}

func TestDrizzleBuilderFor_Postgres(t *testing.T) {
	for dataType, want := range postgresToDrizzleBuilder {
		if got := drizzleBuilderFor(dbengine.DialectPostgres, dataType); got != want {
			t.Errorf("drizzleBuilderFor(postgres, %q) = %q, want %q", dataType, got, want)
		}
	}
}

func TestDrizzleBuilderFor_MySQL(t *testing.T) {
	for dataType, want := range mysqlToDrizzleBuilder {
		if got := drizzleBuilderFor(dbengine.DialectMySQL, dataType); got != want {
			t.Errorf("drizzleBuilderFor(mysql, %q) = %q, want %q", dataType, got, want)
		}
	}
}

func TestDrizzleBuilderFor_UnknownTypeFallsBackToText(t *testing.T) {
	if got := drizzleBuilderFor(dbengine.DialectPostgres, "tsvector"); got != "text" {
		t.Errorf("drizzleBuilderFor(postgres, tsvector) = %q, want text", got)
	}
	if got := drizzleBuilderFor(dbengine.DialectMySQL, "geometry"); got != "text" {
		t.Errorf("drizzleBuilderFor(mysql, geometry) = %q, want text", got)
	}
}
