package export

import (
	"strings"
	"testing"
	"time"
)

func TestToCSV_NullVsEmptyString(t *testing.T) {
	got, err := ToCSV([]string{"id", "note"}, [][]any{
		{1, nil},
		{2, ""},
		{3, "hello"},
	})
	if err != nil {
		t.Fatalf("ToCSV() error = %v", err)
	}

	want := "id,note\r\n" +
		"1,\r\n" +
		"2,\"\"\r\n" +
		"3,hello\r\n"
	if got != want {
		t.Errorf("ToCSV() = %q, want %q", got, want)
	}
}

func TestToCSV_DateValue(t *testing.T) {
	when := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	got, err := ToCSV([]string{"created_at"}, [][]any{{when}})
	if err != nil {
		t.Fatalf("ToCSV() error = %v", err)
	}
	want := "created_at\r\n2024-03-15T10:30:00Z\r\n"
	if got != want {
		t.Errorf("ToCSV() = %q, want %q", got, want)
	}
}

func TestToCSV_NumericValue(t *testing.T) {
	got, err := ToCSV([]string{"price"}, [][]any{{12.5}, {3}})
	if err != nil {
		t.Fatalf("ToCSV() error = %v", err)
	}
	want := "price\r\n12.5\r\n3\r\n"
	if got != want {
		t.Errorf("ToCSV() = %q, want %q", got, want)
	}
}

func TestToCSV_QuotesFieldsWithSpecialCharacters(t *testing.T) {
	got, err := ToCSV([]string{"text"}, [][]any{
		{"has,comma"},
		{"has\"quote"},
		{"has\nnewline"},
	})
	if err != nil {
		t.Fatalf("ToCSV() error = %v", err)
	}
	wantLines := []string{
		`text`,
		`"has,comma"`,
		`"has""quote"`,
		"\"has\nnewline\"",
	}
	want := strings.Join(wantLines, "\r\n") + "\r\n"
	if got != want {
		t.Errorf("ToCSV() = %q, want %q", got, want)
	}
}

func TestToCSV_ShortRowPadsAsNull(t *testing.T) {
	got, err := ToCSV([]string{"a", "b"}, [][]any{{1}})
	if err != nil {
		t.Fatalf("ToCSV() error = %v", err)
	}
	want := "a,b\r\n1,\r\n"
	if got != want {
		t.Errorf("ToCSV() = %q, want %q", got, want)
	}
}
