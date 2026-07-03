package schemaexport

import "testing"

func TestCamelCase(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"widgets", "widgets"},
		{"author_id", "authorId"},
		{"tenant_id", "tenantId"},
		{"id", "id"},
		{"Name", "name"},
	}
	for _, c := range cases {
		if got := camelCase(c.name); got != c.want {
			t.Errorf("camelCase(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
