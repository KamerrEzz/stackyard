package mongo

import (
	"encoding/base64"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// sanitizeDocument converts every value in doc via sanitizeValue, returning
// a new map safe to cross the Wails/JSON bridge — the driver's default
// registry decodes a document target's own nested subdocuments/arrays as
// bson.M/bson.A (confirmed empirically against this project's pinned
// go.mongodb.org/mongo-driver version, not assumed), so sanitizeValue's
// bson.M/bson.A cases are what actually fire at every real nesting depth in
// practice; the map[string]any/[]any/bson.D cases exist as defensive
// coverage for callers that hand this function an already-plain Go value
// (e.g. InsertDocument's returned document, built from caller-supplied
// map[string]any) rather than a value fresh off a cursor.
func sanitizeDocument(doc map[string]any) map[string]any {
	result := make(map[string]any, len(doc))
	for k, v := range doc {
		result[k] = sanitizeValue(v)
	}
	return result
}

// sanitizeArray converts every element of arr via sanitizeValue.
func sanitizeArray(arr []any) []any {
	result := make([]any, len(arr))
	for i, v := range arr {
		result[i] = sanitizeValue(v)
	}
	return result
}

// sanitizeValue recursively converts a single BSON-decoded value into a
// JSON-marshalable, human-readable one:
//
//   - primitive.ObjectID becomes its hex string (mongo.NewObjectID().Hex()) —
//     this is also the exact string form UpdateDocument/DeleteDocuments
//     expect back for their id/ids parameters, so a document's _id round-trips
//     through the frontend as a plain string with no separate encoding scheme.
//   - primitive.DateTime and time.Time become RFC3339Nano UTC strings.
//   - primitive.Timestamp (an internal replication construct, distinct from a
//     user date field) becomes an RFC3339Nano UTC string derived from its
//     seconds component.
//   - primitive.Decimal128 becomes its decimal string form.
//   - primitive.Binary becomes standard base64 text.
//   - primitive.Regex becomes its pattern text (flags are dropped — this is a
//     display/edit convenience, not a lossless round trip).
//   - bson.M/map[string]any and bson.A/[]any/bson.D recurse via
//     sanitizeDocument/sanitizeArray so nested structure at any depth is
//     fully converted, not just the top level.
//   - every other value (string, bool, int32, int64, float64, nil, ...)
//     passes through unchanged — encoding/json already knows how to marshal
//     it.
func sanitizeValue(v any) any {
	switch val := v.(type) {
	case primitive.ObjectID:
		return val.Hex()
	case primitive.DateTime:
		return val.Time().UTC().Format(time.RFC3339Nano)
	case time.Time:
		return val.UTC().Format(time.RFC3339Nano)
	case primitive.Timestamp:
		return time.Unix(int64(val.T), 0).UTC().Format(time.RFC3339Nano)
	case primitive.Decimal128:
		return val.String()
	case primitive.Binary:
		return base64.StdEncoding.EncodeToString(val.Data)
	case primitive.Regex:
		return val.Pattern
	case bson.M:
		return sanitizeDocument(val)
	case map[string]any:
		return sanitizeDocument(val)
	case bson.A:
		return sanitizeArray(val)
	case []any:
		return sanitizeArray(val)
	case bson.D:
		return sanitizeDocument(val.Map())
	default:
		return val
	}
}
