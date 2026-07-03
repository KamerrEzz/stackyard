package export

import "testing"

func TestFormatCSVValue_DriverValuerNumeric(t *testing.T) {
	text, isNull := formatCSVValue(fakeNumeric{text: "99.9900"})
	if isNull {
		t.Error("formatCSVValue(fakeNumeric) reported isNull, want false")
	}
	if text != "99.9900" {
		t.Errorf("formatCSVValue(fakeNumeric) = %q, want the bare numeric text 99.9900", text)
	}
}

func TestFormatCSVValue_Bytes(t *testing.T) {
	text, isNull := formatCSVValue([]byte("raw bytes"))
	if isNull {
		t.Error("formatCSVValue([]byte) reported isNull, want false")
	}
	if text != "raw bytes" {
		t.Errorf("formatCSVValue([]byte) = %q, want %q", text, "raw bytes")
	}
}

func TestFormatCSVValue_Nil(t *testing.T) {
	text, isNull := formatCSVValue(nil)
	if !isNull {
		t.Error("formatCSVValue(nil) reported isNull = false, want true")
	}
	if text != "" {
		t.Errorf("formatCSVValue(nil) text = %q, want empty", text)
	}
}

func TestNormalizeJSONValue_Bytes(t *testing.T) {
	got := normalizeJSONValue([]byte("raw"))
	if got != "raw" {
		t.Errorf("normalizeJSONValue([]byte) = %v, want string \"raw\"", got)
	}
}

func TestNormalizeJSONValue_PassesThroughOtherTypes(t *testing.T) {
	if got := normalizeJSONValue(42); got != 42 {
		t.Errorf("normalizeJSONValue(42) = %v, want 42 unchanged", got)
	}
	if got := normalizeJSONValue(nil); got != nil {
		t.Errorf("normalizeJSONValue(nil) = %v, want nil", got)
	}
}
