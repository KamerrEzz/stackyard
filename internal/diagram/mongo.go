package diagram

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// FieldKind labels the shape of a single field's value as observed in a
// sampled MongoDB document: a scalar BSON/JSON kind, a nested object, or an
// array. Every document handed to InferCollectionShape has already passed
// through this project's BSON-to-JSON-safe conversion (see
// internal/dbengine/mongo/convert.go), so an ObjectID or a date already
// arrives here as a plain string — FieldKind therefore only ever
// distinguishes the kinds Go's own JSON-ish dynamic typing already carries
// (string, number split into int/float, bool, null, object, array), not
// BSON's finer-grained wire types.
type FieldKind string

const (
	FieldKindString  FieldKind = "string"
	FieldKindInt     FieldKind = "int"
	FieldKindFloat   FieldKind = "float"
	FieldKindBool    FieldKind = "bool"
	FieldKindNull    FieldKind = "null"
	FieldKindObject  FieldKind = "object"
	FieldKindArray   FieldKind = "array"
	FieldKindUnknown FieldKind = "unknown"
)

// FieldShape describes one field name as observed across a sample of
// documents from a single collection.
//
// Type variance across the sample is handled by listing every distinct kind
// observed for the field, not by picking the most common one and flagging
// the rest as "mixed" — a field that is a string in some documents and an
// int in others reports Kinds == []FieldKind{FieldKindInt, FieldKindString}
// (sorted for deterministic output), so a viewer sees exactly which types
// disagree rather than an uninformative "mixed" label. This is a deliberate
// choice for a pedagogical tool: the whole point of this feature is showing
// a student where document shapes disagree, not hiding the disagreement
// behind a single word.
type FieldShape struct {
	// Name is the field's key as it appears in the sampled documents.
	Name string

	// Kinds holds every distinct FieldKind observed for this field across
	// the sample, sorted alphabetically for deterministic output.
	Kinds []FieldKind

	// Optional is true when at least one sampled document omitted this
	// field entirely (as opposed to including it with a null value, which
	// is instead reflected as FieldKindNull in Kinds).
	Optional bool

	// Nested is the merged shape of this field's nested object values: for
	// an object-kind field, every sampled object value; for an array-kind
	// field whose elements include objects, every such element across every
	// sampled array. It is nil when the field was never observed as an
	// object, or as an array of objects. If a field is observed as both a
	// direct object and an array of objects across the sample (a rare shape
	// disagreement in its own right), Nested is built from the direct
	// object values only — the array's own element kinds still surface via
	// ElementKinds even in that case.
	Nested *CollectionShape

	// ElementKinds holds every distinct FieldKind observed among this
	// field's array elements, across every sampled document, sorted
	// alphabetically. It is empty when the field was never observed as an
	// array, or every sampled array for it was empty.
	ElementKinds []FieldKind
}

// CollectionShape is the inferred shape of one MongoDB collection (or one
// nested object/array-element position within it): every distinct field
// name observed across a document sample, plus how large that sample was.
// Fields are sorted alphabetically by Name for deterministic Mermaid output
// — Go map iteration order is randomized, and this package's own
// BuildMongoStructureDiagram must produce the same text on every call for
// the same input.
type CollectionShape struct {
	// SampleSize is the number of documents InferCollectionShape examined
	// to produce Fields — the top-level count for a collection's own shape,
	// or the number of nested object/array-element values examined for a
	// Nested shape.
	SampleSize int

	// Fields holds one FieldShape per distinct field name observed anywhere
	// in the sample, sorted alphabetically by Name.
	Fields []FieldShape
}

// InferCollectionShape examines documents (a random sample from one
// collection, e.g. via dbenginemongo.Engine.SampleDocuments) and infers its
// shape: every field name observed, the type(s) observed for it, whether it
// was present in every document, and — for a nested object or an array of
// objects — the recursively inferred shape of that nested structure (tasks.md
// 5.6, spec.md §4.11). An empty or nil documents slice returns a
// CollectionShape with SampleSize 0 and no fields, not an error — an empty
// sample is a legitimate (if uninformative) input, not a failure.
func InferCollectionShape(documents []map[string]any) CollectionShape {
	valuesByField := make(map[string][]any)
	for _, doc := range documents {
		for name, value := range doc {
			valuesByField[name] = append(valuesByField[name], value)
		}
	}

	names := make([]string, 0, len(valuesByField))
	for name := range valuesByField {
		names = append(names, name)
	}
	sort.Strings(names)

	fields := make([]FieldShape, 0, len(names))
	for _, name := range names {
		fields = append(fields, inferFieldShape(name, valuesByField[name], len(documents)))
	}

	return CollectionShape{SampleSize: len(documents), Fields: fields}
}

// inferFieldShape builds the FieldShape for one field name given every
// value observed for it (one entry per document that had the key present)
// and totalDocuments (the full sample size, used to detect optionality).
func inferFieldShape(name string, values []any, totalDocuments int) FieldShape {
	observedKinds := make(map[FieldKind]bool)
	var objectValues []map[string]any
	var arrayElements []any

	for _, value := range values {
		kind := kindOf(value)
		observedKinds[kind] = true

		switch typed := value.(type) {
		case map[string]any:
			objectValues = append(objectValues, typed)
		case []any:
			arrayElements = append(arrayElements, typed...)
		}
	}

	shape := FieldShape{
		Name:     name,
		Kinds:    sortedKinds(observedKinds),
		Optional: len(values) < totalDocuments,
	}

	if len(objectValues) > 0 {
		nested := InferCollectionShape(objectValues)
		shape.Nested = &nested
	}

	if observedKinds[FieldKindArray] {
		elementKinds := make(map[FieldKind]bool)
		var elementObjects []map[string]any
		for _, element := range arrayElements {
			kind := kindOf(element)
			elementKinds[kind] = true
			if obj, ok := element.(map[string]any); ok {
				elementObjects = append(elementObjects, obj)
			}
		}
		shape.ElementKinds = sortedKinds(elementKinds)

		if shape.Nested == nil && len(elementObjects) > 0 {
			nested := InferCollectionShape(elementObjects)
			shape.Nested = &nested
		}
	}

	return shape
}

// kindOf classifies a single already-BSON-sanitized value into a FieldKind.
// FieldKindUnknown is a defensive fallback that should never actually fire
// against output from internal/dbengine/mongo's sanitizeValue, which only
// ever hands back the types this switch already covers.
func kindOf(value any) FieldKind {
	switch value.(type) {
	case nil:
		return FieldKindNull
	case string:
		return FieldKindString
	case bool:
		return FieldKindBool
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return FieldKindInt
	case float32, float64:
		return FieldKindFloat
	case map[string]any:
		return FieldKindObject
	case []any:
		return FieldKindArray
	default:
		return FieldKindUnknown
	}
}

// sortedKinds returns every FieldKind present in observed, sorted
// alphabetically for deterministic output.
func sortedKinds(observed map[FieldKind]bool) []FieldKind {
	kinds := make([]FieldKind, 0, len(observed))
	for kind := range observed {
		kinds = append(kinds, kind)
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })
	return kinds
}

// BuildMongoStructureDiagram renders collections (keyed by collection name)
// as Mermaid erDiagram text, one entity block per collection with no
// relationship lines between them — MongoDB has no foreign keys for this
// function to derive a relationship from, matching spec.md §4.11's "no real
// foreign keys" acceptance criterion for the MongoDB structure diagram. This
// package does NOT attempt heuristic relationship detection (e.g. treating
// a field named "xId" as an implied reference to collection "x") — that was
// explicitly left as an optional, skippable stretch goal for this feature,
// and it is skipped here so every relationship-shaped line in the output
// stays honestly limited to what was actually observed, not guessed.
//
// The output always starts with a Mermaid comment banner stating the
// diagram is an inferred structure, not an enforced relationship, and one
// more per-collection comment line names each collection's sample size —
// both survive into the raw copyable Mermaid text export (tasks.md 5.6,
// spec.md §4.11's "same export capabilities as the relational ER diagram"),
// not just an on-screen badge that a pasted-elsewhere copy would lose.
//
// collections is a map (not a slice) because collection shapes are
// naturally keyed by name with no other ordering signal from MongoDB
// itself; this function sorts the keys before writing output so repeated
// calls with the same input always produce byte-identical Mermaid text,
// since Go's own map iteration order is randomized per run.
func BuildMongoStructureDiagram(collections map[string]CollectionShape) string {
	names := make([]string, 0, len(collections))
	for name := range collections {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(inferredStructureBanner + "\n")
	b.WriteString("erDiagram\n")
	for _, name := range names {
		writeCollectionEntityBlock(&b, name, collections[name])
	}
	return b.String()
}

// inferredStructureBanner is the Mermaid comment line prefixed to every
// generated MongoDB structure diagram (see BuildMongoStructureDiagram's doc
// comment for why this lives inside the Mermaid text itself, not only in a
// frontend badge). Its wording deliberately echoes spec.md §4.11's own
// phrase verbatim ("inferred structure, not an enforced relationship") so
// the diagram's own text and the product spec never drift apart.
const inferredStructureBanner = "%% Inferred structure - not an enforced relationship " +
	"(MongoDB has no schema or foreign keys; each collection's shape below " +
	"is inferred from a sample of its documents)"

// writeCollectionEntityBlock appends one Mermaid comment line naming
// collectionName's sample size, followed by its entity block: one attribute
// line per top-level field, with a nested object or array-of-objects field's
// own fields flattened into additional attribute lines directly beneath it
// (see writeFieldLine).
func writeCollectionEntityBlock(b *strings.Builder, collectionName string, shape CollectionShape) {
	fmt.Fprintf(b, "    %%%% %s: shape inferred from %d sampled document(s)\n", collectionName, shape.SampleSize)
	fmt.Fprintf(b, "    %s {\n", mermaidToken(collectionName))
	writeFieldLines(b, "", shape.Fields)
	b.WriteString("    }\n")
}

// writeFieldLines appends one attribute line per field in fields, recursing
// into any nested shape with prefix extended by that field's own name.
func writeFieldLines(b *strings.Builder, prefix string, fields []FieldShape) {
	for _, field := range fields {
		writeFieldLine(b, prefix, field)
	}
}

// writeFieldLine appends one Mermaid erDiagram attribute line for field,
// named prefix+field.Name (mongoFieldToken-sanitized) and typed by every
// kind observed for it, joined with "_or_" when more than one kind was
// observed (see FieldShape's own doc comment for why type variance is
// listed in full rather than collapsed to "mixed"). An optional field, or
// one observed as an array, carries a trailing quoted Mermaid comment
// noting that — genuine Mermaid erDiagram syntax (`type name "comment"`),
// not a made-up extension. A nested shape's own fields are then written
// directly beneath it with prefix extended by field.Name + "_", since
// Mermaid's erDiagram entity blocks have no native nested-attribute syntax
// to represent a subdocument any other way while still keeping every field
// inside one flat, valid entity block.
func writeFieldLine(b *strings.Builder, prefix string, field FieldShape) {
	attrName := mongoFieldToken(prefix + field.Name)
	typeToken := mermaidToken(kindsToken(field.Kinds))
	comment := fieldComment(field)

	if comment != "" {
		fmt.Fprintf(b, "        %s %s \"%s\"\n", typeToken, attrName, comment)
	} else {
		fmt.Fprintf(b, "        %s %s\n", typeToken, attrName)
	}

	if field.Nested != nil {
		writeFieldLines(b, prefix+field.Name+"_", field.Nested.Fields)
	}
}

// kindsToken joins kinds (already sorted by sortedKinds) with "_or_" into a
// single Mermaid-safe type token, e.g. []FieldKind{"int","string"} becomes
// "int_or_string". An empty kinds slice (never expected in practice, since
// every field in a FieldShape was observed at least once) falls back to
// "unknown" rather than emitting a blank type token.
func kindsToken(kinds []FieldKind) string {
	if len(kinds) == 0 {
		return string(FieldKindUnknown)
	}
	parts := make([]string, len(kinds))
	for i, kind := range kinds {
		parts[i] = string(kind)
	}
	return strings.Join(parts, "_or_")
}

// fieldComment builds field's trailing quoted Mermaid comment: "optional"
// when the field wasn't present in every sampled document, and, for a field
// observed as an array, "array of <element kinds>" (or "array of unknown
// (empty sample)" when every sampled array for it was empty). Both notes
// are joined with "; " when both apply. An empty return means no comment is
// written at all.
func fieldComment(field FieldShape) string {
	var notes []string
	if field.Optional {
		notes = append(notes, "optional")
	}
	for _, kind := range field.Kinds {
		if kind == FieldKindArray {
			if len(field.ElementKinds) == 0 {
				notes = append(notes, "array of unknown (empty sample)")
			} else {
				notes = append(notes, "array of "+kindsToken(field.ElementKinds))
			}
			break
		}
	}
	return strings.Join(notes, "; ")
}

// mongoFieldToken makes name safe to use as a Mermaid erDiagram attribute
// name for a field observed in a sampled MongoDB document: every run of
// characters that isn't a letter, digit, or underscore collapses to a
// single underscore, mirroring mermaidToken's own collapsing rule in
// relational.go. Unlike mermaidToken, a leading or trailing underscore is
// preserved rather than trimmed — MongoDB field names commonly and
// meaningfully start with one (_id being the obvious case; trimming it to
// "id" would misrepresent the field actually present in the data, which
// matters more here than in the relational diagram, where SQL column names
// essentially never start with an underscore). An input that collapses to
// nothing but underscores (including the empty string) falls back to
// "field" rather than emitting a bare "_" attribute name.
func mongoFieldToken(name string) string {
	var b strings.Builder
	lastWasUnderscore := false
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
			lastWasUnderscore = false
			continue
		}
		if !lastWasUnderscore {
			b.WriteRune('_')
			lastWasUnderscore = true
		}
	}
	token := b.String()
	if strings.Trim(token, "_") == "" {
		return "field"
	}
	return token
}
