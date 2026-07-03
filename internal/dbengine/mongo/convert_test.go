package mongo

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestSanitizeValue_ObjectIDBecomesHexString(t *testing.T) {
	id := primitive.NewObjectID()

	got := sanitizeValue(id)

	hex, ok := got.(string)
	if !ok {
		t.Fatalf("sanitizeValue(ObjectID) = %#v (%T), want a string", got, got)
	}
	if hex != id.Hex() {
		t.Errorf("sanitizeValue(ObjectID) = %q, want %q", hex, id.Hex())
	}
}

func TestSanitizeValue_DateTimeBecomesRFC3339String(t *testing.T) {
	now := time.Date(2026, 7, 2, 15, 4, 5, 0, time.UTC)
	dt := primitive.NewDateTimeFromTime(now)

	got := sanitizeValue(dt)

	str, ok := got.(string)
	if !ok {
		t.Fatalf("sanitizeValue(DateTime) = %#v (%T), want a string", got, got)
	}
	parsed, err := time.Parse(time.RFC3339Nano, str)
	if err != nil {
		t.Fatalf("sanitizeValue(DateTime) = %q, not parseable as RFC3339: %v", str, err)
	}
	if !parsed.Equal(now) {
		t.Errorf("sanitizeValue(DateTime) round-trips to %v, want %v", parsed, now)
	}
}

func TestSanitizeValue_TimeTimeBecomesRFC3339String(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got := sanitizeValue(now)

	str, ok := got.(string)
	if !ok {
		t.Fatalf("sanitizeValue(time.Time) = %#v (%T), want a string", got, got)
	}
	if str != now.Format(time.RFC3339Nano) {
		t.Errorf("sanitizeValue(time.Time) = %q, want %q", str, now.Format(time.RFC3339Nano))
	}
}

func TestSanitizeValue_Decimal128BecomesDecimalString(t *testing.T) {
	dec, err := primitive.ParseDecimal128("19.99")
	if err != nil {
		t.Fatalf("ParseDecimal128 setup failed: %v", err)
	}

	got := sanitizeValue(dec)

	str, ok := got.(string)
	if !ok {
		t.Fatalf("sanitizeValue(Decimal128) = %#v (%T), want a string", got, got)
	}
	if str != "19.99" {
		t.Errorf("sanitizeValue(Decimal128) = %q, want %q", str, "19.99")
	}
}

func TestSanitizeValue_BinaryBecomesBase64String(t *testing.T) {
	data := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	bin := primitive.Binary{Subtype: 0x00, Data: data}

	got := sanitizeValue(bin)

	str, ok := got.(string)
	if !ok {
		t.Fatalf("sanitizeValue(Binary) = %#v (%T), want a string", got, got)
	}
	if str != base64.StdEncoding.EncodeToString(data) {
		t.Errorf("sanitizeValue(Binary) = %q, want %q", str, base64.StdEncoding.EncodeToString(data))
	}
}

func TestSanitizeValue_RegexBecomesPatternString(t *testing.T) {
	re := primitive.Regex{Pattern: "^abc$", Options: "i"}

	got := sanitizeValue(re)

	if got != "^abc$" {
		t.Errorf("sanitizeValue(Regex) = %v, want %q", got, "^abc$")
	}
}

func TestSanitizeValue_ScalarsPassThroughUnchanged(t *testing.T) {
	cases := []any{"plain string", true, false, int32(42), int64(42), float64(3.14), nil}
	for _, c := range cases {
		got := sanitizeValue(c)
		if got != c {
			t.Errorf("sanitizeValue(%#v) = %#v, want it unchanged", c, got)
		}
	}
}

func TestSanitizeDocument_NestedObjectsAndArraysRecurseAndConvertObjectIDsAndDates(t *testing.T) {
	nestedID := primitive.NewObjectID()
	when := primitive.NewDateTimeFromTime(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))

	doc := map[string]any{
		"name": "widget",
		"nested": bson.M{
			"ownerId": nestedID,
			"tags":    bson.A{"a", "b", bson.M{"deep": true, "when": when}},
		},
		"list": []any{1, 2, bson.M{"x": nestedID}},
	}

	got := sanitizeDocument(doc)

	nested, ok := got["nested"].(map[string]any)
	if !ok {
		t.Fatalf("sanitizeDocument()[\"nested\"] = %#v (%T), want map[string]any", got["nested"], got["nested"])
	}
	if nested["ownerId"] != nestedID.Hex() {
		t.Errorf("nested.ownerId = %v, want %q", nested["ownerId"], nestedID.Hex())
	}

	tags, ok := nested["tags"].([]any)
	if !ok || len(tags) != 3 {
		t.Fatalf("nested.tags = %#v, want a 3-element []any", nested["tags"])
	}
	deep, ok := tags[2].(map[string]any)
	if !ok {
		t.Fatalf("nested.tags[2] = %#v (%T), want map[string]any", tags[2], tags[2])
	}
	if deep["when"] != when.Time().UTC().Format(time.RFC3339Nano) {
		t.Errorf("nested.tags[2].when = %v, want an RFC3339 string", deep["when"])
	}

	list, ok := got["list"].([]any)
	if !ok || len(list) != 3 {
		t.Fatalf("sanitizeDocument()[\"list\"] = %#v, want a 3-element []any", got["list"])
	}
	inner, ok := list[2].(map[string]any)
	if !ok || inner["x"] != nestedID.Hex() {
		t.Errorf("list[2] = %#v, want {x: %q}", list[2], nestedID.Hex())
	}
}

func TestSanitizeDocument_ResultIsJSONMarshalable(t *testing.T) {
	doc := map[string]any{
		"_id":   primitive.NewObjectID(),
		"when":  primitive.NewDateTimeFromTime(time.Now()),
		"count": int64(5),
		"nested": bson.M{
			"arr": bson.A{1, "two", bson.M{"three": 3}},
		},
	}

	sanitized := sanitizeDocument(doc)

	if _, err := json.Marshal(sanitized); err != nil {
		t.Fatalf("json.Marshal(sanitizeDocument(doc)) failed: %v", err)
	}
}

func TestSanitizeDocument_BSONDRecurses(t *testing.T) {
	id := primitive.NewObjectID()
	doc := map[string]any{
		"nested": bson.D{{Key: "ownerId", Value: id}, {Key: "n", Value: int32(7)}},
	}

	got := sanitizeDocument(doc)

	nested, ok := got["nested"].(map[string]any)
	if !ok {
		t.Fatalf("sanitizeDocument()[\"nested\"] = %#v (%T), want map[string]any", got["nested"], got["nested"])
	}
	if nested["ownerId"] != id.Hex() {
		t.Errorf("nested.ownerId = %v, want %q", nested["ownerId"], id.Hex())
	}
}
