package diagram

import (
	"reflect"
	"testing"
)

func TestInferCollectionShape_ConsistentTypesAcrossSample(t *testing.T) {
	documents := []map[string]any{
		{"name": "bolt", "price": 1.5, "inStock": true},
		{"name": "nut", "price": 2.0, "inStock": false},
	}

	got := InferCollectionShape(documents)

	if got.SampleSize != 2 {
		t.Fatalf("SampleSize = %d, want 2", got.SampleSize)
	}
	if len(got.Fields) != 3 {
		t.Fatalf("len(Fields) = %d, want 3, got %+v", len(got.Fields), got.Fields)
	}

	want := []FieldShape{
		{Name: "inStock", Kinds: []FieldKind{FieldKindBool}},
		{Name: "name", Kinds: []FieldKind{FieldKindString}},
		{Name: "price", Kinds: []FieldKind{FieldKindFloat}},
	}
	for i, w := range want {
		g := got.Fields[i]
		if g.Name != w.Name || !reflect.DeepEqual(g.Kinds, w.Kinds) || g.Optional {
			t.Errorf("Fields[%d] = %+v, want %+v with Optional=false", i, g, w)
		}
	}
}

func TestInferCollectionShape_TypeVarianceListsEveryObservedKind(t *testing.T) {
	documents := []map[string]any{
		{"sku": "abc"},
		{"sku": 123},
	}

	got := InferCollectionShape(documents)

	if len(got.Fields) != 1 {
		t.Fatalf("len(Fields) = %d, want 1", len(got.Fields))
	}
	field := got.Fields[0]
	want := []FieldKind{FieldKindInt, FieldKindString}
	if !reflect.DeepEqual(field.Kinds, want) {
		t.Errorf("sku.Kinds = %v, want %v (both observed types listed, not collapsed to \"mixed\")", field.Kinds, want)
	}
	if field.Optional {
		t.Error("sku.Optional = true, want false (present in every document)")
	}
}

func TestInferCollectionShape_FieldMissingFromSomeDocumentsIsOptional(t *testing.T) {
	documents := []map[string]any{
		{"name": "bolt", "notes": "shiny"},
		{"name": "nut"},
	}

	got := InferCollectionShape(documents)

	fieldsByName := fieldMap(got.Fields)

	if fieldsByName["name"].Optional {
		t.Error("name.Optional = true, want false (present in every document)")
	}
	if !fieldsByName["notes"].Optional {
		t.Error("notes.Optional = false, want true (absent from one document)")
	}
}

func TestInferCollectionShape_ExplicitNullIsNotTreatedAsAbsent(t *testing.T) {
	documents := []map[string]any{
		{"name": "bolt", "deletedAt": nil},
		{"name": "nut", "deletedAt": nil},
	}

	got := InferCollectionShape(documents)

	field := fieldMap(got.Fields)["deletedAt"]
	if field.Optional {
		t.Error("deletedAt.Optional = true, want false (key present with a null value in every document)")
	}
	if !reflect.DeepEqual(field.Kinds, []FieldKind{FieldKindNull}) {
		t.Errorf("deletedAt.Kinds = %v, want [null]", field.Kinds)
	}
}

func TestInferCollectionShape_NestedObjectFieldRecursesIntoSubShape(t *testing.T) {
	documents := []map[string]any{
		{"name": "bolt", "address": map[string]any{"city": "NYC", "zip": "10001"}},
		{"name": "nut", "address": map[string]any{"city": "LA", "zip": 90001}},
	}

	got := InferCollectionShape(documents)

	address := fieldMap(got.Fields)["address"]
	if !reflect.DeepEqual(address.Kinds, []FieldKind{FieldKindObject}) {
		t.Fatalf("address.Kinds = %v, want [object]", address.Kinds)
	}
	if address.Nested == nil {
		t.Fatal("address.Nested = nil, want a recursively inferred sub-shape")
	}
	if address.Nested.SampleSize != 2 {
		t.Errorf("address.Nested.SampleSize = %d, want 2", address.Nested.SampleSize)
	}

	nestedFields := fieldMap(address.Nested.Fields)
	if !reflect.DeepEqual(nestedFields["city"].Kinds, []FieldKind{FieldKindString}) {
		t.Errorf("address.city.Kinds = %v, want [string]", nestedFields["city"].Kinds)
	}
	if !reflect.DeepEqual(nestedFields["zip"].Kinds, []FieldKind{FieldKindInt, FieldKindString}) {
		t.Errorf("address.zip.Kinds = %v, want [int, string] (a string in one document, an int in the other)", nestedFields["zip"].Kinds)
	}
}

func TestInferCollectionShape_ArrayOfScalarsReportsElementKinds(t *testing.T) {
	documents := []map[string]any{
		{"name": "bolt", "tags": []any{"a", "b"}},
		{"name": "nut", "tags": []any{"c"}},
	}

	got := InferCollectionShape(documents)

	tags := fieldMap(got.Fields)["tags"]
	if !reflect.DeepEqual(tags.Kinds, []FieldKind{FieldKindArray}) {
		t.Fatalf("tags.Kinds = %v, want [array]", tags.Kinds)
	}
	if !reflect.DeepEqual(tags.ElementKinds, []FieldKind{FieldKindString}) {
		t.Errorf("tags.ElementKinds = %v, want [string]", tags.ElementKinds)
	}
	if tags.Nested != nil {
		t.Error("tags.Nested != nil, want nil for an array of scalars")
	}
}

func TestInferCollectionShape_ArrayOfObjectsRecursesIntoElementShape(t *testing.T) {
	documents := []map[string]any{
		{"items": []any{
			map[string]any{"sku": "a"},
			map[string]any{"sku": "b"},
		}},
	}

	got := InferCollectionShape(documents)

	items := fieldMap(got.Fields)["items"]
	if !reflect.DeepEqual(items.ElementKinds, []FieldKind{FieldKindObject}) {
		t.Fatalf("items.ElementKinds = %v, want [object]", items.ElementKinds)
	}
	if items.Nested == nil {
		t.Fatal("items.Nested = nil, want a shape inferred from the array's element objects")
	}
	if items.Nested.SampleSize != 2 {
		t.Errorf("items.Nested.SampleSize = %d, want 2 (one per element object across the whole sample)", items.Nested.SampleSize)
	}
	skuKinds := fieldMap(items.Nested.Fields)["sku"].Kinds
	if !reflect.DeepEqual(skuKinds, []FieldKind{FieldKindString}) {
		t.Errorf("items[].sku.Kinds = %v, want [string]", skuKinds)
	}
}

func TestInferCollectionShape_EmptySampleReturnsNoFields(t *testing.T) {
	got := InferCollectionShape(nil)

	if got.SampleSize != 0 {
		t.Errorf("SampleSize = %d, want 0", got.SampleSize)
	}
	if len(got.Fields) != 0 {
		t.Errorf("len(Fields) = %d, want 0", len(got.Fields))
	}
}

func fieldMap(fields []FieldShape) map[string]FieldShape {
	m := make(map[string]FieldShape, len(fields))
	for _, f := range fields {
		m[f.Name] = f
	}
	return m
}

func TestBuildMongoStructureDiagram_SingleCollectionSimpleFields(t *testing.T) {
	shapes := map[string]CollectionShape{
		"widgets": {
			SampleSize: 2,
			Fields: []FieldShape{
				{Name: "name", Kinds: []FieldKind{FieldKindString}},
				{Name: "price", Kinds: []FieldKind{FieldKindFloat}},
			},
		},
	}

	want := inferredStructureBanner + "\n" +
		"erDiagram\n" +
		"    %% widgets: shape inferred from 2 sampled document(s)\n" +
		"    widgets {\n" +
		"        string name\n" +
		"        float price\n" +
		"    }\n"

	if got := BuildMongoStructureDiagram(shapes); got != want {
		t.Errorf("BuildMongoStructureDiagram() =\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildMongoStructureDiagram_NestedObjectOptionalFieldAndArray(t *testing.T) {
	shapes := map[string]CollectionShape{
		"users": {
			SampleSize: 3,
			Fields: []FieldShape{
				{Name: "_id", Kinds: []FieldKind{FieldKindString}},
				{
					Name:  "address",
					Kinds: []FieldKind{FieldKindObject},
					Nested: &CollectionShape{
						SampleSize: 3,
						Fields: []FieldShape{
							{Name: "city", Kinds: []FieldKind{FieldKindString}},
							{Name: "zip", Kinds: []FieldKind{FieldKindInt, FieldKindString}},
						},
					},
				},
				{Name: "age", Kinds: []FieldKind{FieldKindInt, FieldKindString}, Optional: true},
				{Name: "tags", Kinds: []FieldKind{FieldKindArray}, ElementKinds: []FieldKind{FieldKindString}},
			},
		},
	}

	want := inferredStructureBanner + "\n" +
		"erDiagram\n" +
		"    %% users: shape inferred from 3 sampled document(s)\n" +
		"    users {\n" +
		"        string _id\n" +
		"        object address\n" +
		"        string address_city\n" +
		"        int_or_string address_zip\n" +
		"        int_or_string age \"optional\"\n" +
		"        array tags \"array of string\"\n" +
		"    }\n"

	if got := BuildMongoStructureDiagram(shapes); got != want {
		t.Errorf("BuildMongoStructureDiagram() =\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildMongoStructureDiagram_EmptyCollectionStillRendersAsAStandaloneEntity(t *testing.T) {
	shapes := map[string]CollectionShape{
		"logs": {SampleSize: 0, Fields: []FieldShape{}},
	}

	want := inferredStructureBanner + "\n" +
		"erDiagram\n" +
		"    %% logs: shape inferred from 0 sampled document(s)\n" +
		"    logs {\n" +
		"    }\n"

	if got := BuildMongoStructureDiagram(shapes); got != want {
		t.Errorf("BuildMongoStructureDiagram() =\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildMongoStructureDiagram_MultipleCollectionsSortedAlphabetically(t *testing.T) {
	shapes := map[string]CollectionShape{
		"zebra": {SampleSize: 1, Fields: []FieldShape{{Name: "id", Kinds: []FieldKind{FieldKindString}}}},
		"apple": {SampleSize: 1, Fields: []FieldShape{{Name: "id", Kinds: []FieldKind{FieldKindString}}}},
	}

	want := inferredStructureBanner + "\n" +
		"erDiagram\n" +
		"    %% apple: shape inferred from 1 sampled document(s)\n" +
		"    apple {\n" +
		"        string id\n" +
		"    }\n" +
		"    %% zebra: shape inferred from 1 sampled document(s)\n" +
		"    zebra {\n" +
		"        string id\n" +
		"    }\n"

	if got := BuildMongoStructureDiagram(shapes); got != want {
		t.Errorf("BuildMongoStructureDiagram() =\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildMongoStructureDiagram_NoCollections(t *testing.T) {
	want := inferredStructureBanner + "\n" + "erDiagram\n"
	if got := BuildMongoStructureDiagram(nil); got != want {
		t.Errorf("BuildMongoStructureDiagram(nil) = %q, want %q", got, want)
	}
}

func TestMongoFieldToken(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"already safe", "widgets", "widgets"},
		{"leading underscore preserved", "_id", "_id"},
		{"double leading underscore preserved", "__v", "__v"},
		{"internal punctuation collapses", "user name", "user_name"},
		{"nested prefix stays intact", "address_city", "address_city"},
		{"all punctuation falls back", "***", "field"},
		{"empty string falls back", "", "field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mongoFieldToken(tc.in); got != tc.want {
				t.Errorf("mongoFieldToken(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestKindsToken(t *testing.T) {
	cases := []struct {
		name string
		in   []FieldKind
		want string
	}{
		{"single kind", []FieldKind{FieldKindString}, "string"},
		{"two kinds joined", []FieldKind{FieldKindInt, FieldKindString}, "int_or_string"},
		{"empty falls back to unknown", nil, "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := kindsToken(tc.in); got != tc.want {
				t.Errorf("kindsToken(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
