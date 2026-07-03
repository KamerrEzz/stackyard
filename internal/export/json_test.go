package export

import (
	"encoding/json"
	"testing"
	"time"
)

func TestToJSON_NullVsEmptyString(t *testing.T) {
	got, err := ToJSON([]string{"id", "note"}, [][]any{
		{1, nil},
		{2, ""},
	})
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	var decoded []map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("json.Unmarshal(ToJSON() output) error = %v; output was %s", err, got)
	}
	if len(decoded) != 2 {
		t.Fatalf("decoded %d rows, want 2", len(decoded))
	}
	if decoded[0]["note"] != nil {
		t.Errorf("row 0 note = %v, want nil (JSON null)", decoded[0]["note"])
	}
	if decoded[1]["note"] != "" {
		t.Errorf("row 1 note = %v, want empty string", decoded[1]["note"])
	}
}

func TestToJSON_PreservesColumnOrder(t *testing.T) {
	got, err := ToJSON([]string{"z_col", "a_col"}, [][]any{{1, 2}})
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}
	zIndex := indexOf(got, `"z_col"`)
	aIndex := indexOf(got, `"a_col"`)
	if zIndex < 0 || aIndex < 0 || zIndex > aIndex {
		t.Errorf("ToJSON() = %s, want z_col key to appear before a_col key (source column order preserved)", got)
	}
}

func TestToJSON_DateValue(t *testing.T) {
	when := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	got, err := ToJSON([]string{"created_at"}, [][]any{{when}})
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("json.Unmarshal error = %v", err)
	}
	if decoded[0]["created_at"] != "2024-03-15T10:30:00Z" {
		t.Errorf("created_at = %v, want RFC3339 string", decoded[0]["created_at"])
	}
}

func TestToJSON_NumericValue(t *testing.T) {
	got, err := ToJSON([]string{"price"}, [][]any{{12.5}})
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("json.Unmarshal error = %v", err)
	}
	if decoded[0]["price"] != 12.5 {
		t.Errorf("price = %v, want 12.5", decoded[0]["price"])
	}
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
