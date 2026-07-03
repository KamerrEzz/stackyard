package snippettemplates

import (
	"strings"
	"testing"

	"stackyard/internal/dbengine"
	"stackyard/internal/storage"
)

func TestList_ReturnsNonEmptyDistinctTemplates(t *testing.T) {
	result := List()
	if len(result) == 0 {
		t.Fatal("List() returned no templates, want at least one")
	}

	seenIDs := make(map[string]bool)
	for _, tmpl := range result {
		if strings.TrimSpace(tmpl.ID) == "" {
			t.Errorf("template %q has an empty ID", tmpl.Name)
		}
		if seenIDs[tmpl.ID] {
			t.Errorf("duplicate template ID %q", tmpl.ID)
		}
		seenIDs[tmpl.ID] = true

		if strings.TrimSpace(tmpl.Name) == "" {
			t.Errorf("template %q has an empty Name", tmpl.ID)
		}
		if strings.TrimSpace(tmpl.Description) == "" {
			t.Errorf("template %q has an empty Description", tmpl.ID)
		}
	}
}

func TestList_ReturnsDefensiveCopy(t *testing.T) {
	first := List()
	if len(first) == 0 {
		t.Fatal("List() returned no templates")
	}
	first[0].ID = "mutated"

	second := List()
	if second[0].ID == "mutated" {
		t.Fatal("List() leaked its internal slice — mutating one result's slice affected a later call")
	}
}

func TestList_EveryTemplateHasBothEngineVariants(t *testing.T) {
	for _, tmpl := range List() {
		for _, engine := range []storage.Engine{storage.EnginePostgres, storage.EngineMySQL} {
			sql, ok := tmpl.SQL[engine]
			if !ok || strings.TrimSpace(sql) == "" {
				t.Errorf("template %q has no SQL for engine %q", tmpl.ID, engine)
			}
		}
	}
}

// TestList_EveryVariantIsSyntacticallyPlausible is a structural check, not a
// real SQL parse — it can't catch every possible dialect error, only obvious
// ones (an empty statement, unbalanced parentheses, no CREATE TABLE at all).
// TestIntegration_SnippetTemplates (templates_integration_test.go, //go:build
// integration) is the real proof: it runs every one of these strings against
// a live Postgres and a live MySQL container.
func TestList_EveryVariantIsSyntacticallyPlausible(t *testing.T) {
	for _, tmpl := range List() {
		for engine, sql := range tmpl.SQL {
			statements := dbengine.SplitStatements(sql)
			if len(statements) == 0 {
				t.Errorf("template %q (%s): SplitStatements found no statements", tmpl.ID, engine)
				continue
			}
			for i, stmt := range statements {
				upper := strings.ToUpper(stmt)
				if !strings.HasPrefix(upper, "CREATE TABLE") {
					t.Errorf("template %q (%s) statement %d does not start with CREATE TABLE: %q", tmpl.ID, engine, i, stmt)
				}
				if strings.Count(stmt, "(") != strings.Count(stmt, ")") {
					t.Errorf("template %q (%s) statement %d has unbalanced parentheses: %q", tmpl.ID, engine, i, stmt)
				}
			}
		}
	}
}
