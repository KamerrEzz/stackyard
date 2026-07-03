package dbengine

import "testing"

func strPtr(s string) *string { return &s }

func TestBuildCreateTableDDL(t *testing.T) {
	tests := []struct {
		name    string
		dialect Dialect
		schema  string
		table   string
		columns []ColumnDefinition
		want    string
	}{
		{
			name:    "simple table with a few columns of different types, Postgres",
			dialect: DialectPostgres,
			schema:  "public",
			table:   "widgets",
			columns: []ColumnDefinition{
				{Name: "name", Type: ColumnTypeText, Nullable: false},
				{Name: "weight", Type: ColumnTypeNumeric, Nullable: true},
				{Name: "active", Type: ColumnTypeBoolean, Nullable: false},
			},
			want: "CREATE TABLE \"public\".\"widgets\" (\n" +
				"  \"name\" TEXT NOT NULL,\n" +
				"  \"weight\" NUMERIC,\n" +
				"  \"active\" BOOLEAN NOT NULL\n" +
				")",
		},
		{
			name:    "simple table with a few columns of different types, MySQL",
			dialect: DialectMySQL,
			schema:  "",
			table:   "widgets",
			columns: []ColumnDefinition{
				{Name: "name", Type: ColumnTypeVarchar, Nullable: false},
				{Name: "weight", Type: ColumnTypeNumeric, Nullable: true},
				{Name: "active", Type: ColumnTypeBoolean, Nullable: false},
			},
			want: "CREATE TABLE `widgets` (\n" +
				"  `name` VARCHAR(255) NOT NULL,\n" +
				"  `weight` DECIMAL,\n" +
				"  `active` BOOLEAN NOT NULL\n" +
				")",
		},
		{
			name:    "table with an auto-increment primary key, Postgres",
			dialect: DialectPostgres,
			schema:  "public",
			table:   "widgets",
			columns: []ColumnDefinition{
				{Name: "id", Type: ColumnTypeSerial, Nullable: false, IsPrimaryKey: true},
				{Name: "name", Type: ColumnTypeText, Nullable: false},
			},
			want: "CREATE TABLE \"public\".\"widgets\" (\n" +
				"  \"id\" SERIAL NOT NULL,\n" +
				"  \"name\" TEXT NOT NULL,\n" +
				"  PRIMARY KEY (\"id\")\n" +
				")",
		},
		{
			name:    "table with an auto-increment primary key, MySQL",
			dialect: DialectMySQL,
			schema:  "shop",
			table:   "widgets",
			columns: []ColumnDefinition{
				{Name: "id", Type: ColumnTypeBigSerial, Nullable: false, IsPrimaryKey: true},
				{Name: "name", Type: ColumnTypeVarchar, Nullable: false},
			},
			want: "CREATE TABLE `shop`.`widgets` (\n" +
				"  `id` BIGINT NOT NULL AUTO_INCREMENT,\n" +
				"  `name` VARCHAR(255) NOT NULL,\n" +
				"  PRIMARY KEY (`id`)\n" +
				")",
		},
		{
			name:    "table with a non-auto-increment primary key",
			dialect: DialectPostgres,
			schema:  "public",
			table:   "settings",
			columns: []ColumnDefinition{
				{Name: "key", Type: ColumnTypeText, Nullable: false, IsPrimaryKey: true},
				{Name: "value", Type: ColumnTypeText, Nullable: true},
			},
			want: "CREATE TABLE \"public\".\"settings\" (\n" +
				"  \"key\" TEXT NOT NULL,\n" +
				"  \"value\" TEXT,\n" +
				"  PRIMARY KEY (\"key\")\n" +
				")",
		},
		{
			name:    "table with a nullable column",
			dialect: DialectPostgres,
			schema:  "",
			table:   "notes",
			columns: []ColumnDefinition{
				{Name: "id", Type: ColumnTypeInteger, Nullable: false},
				{Name: "body", Type: ColumnTypeText, Nullable: true},
			},
			want: "CREATE TABLE \"notes\" (\n" +
				"  \"id\" INTEGER NOT NULL,\n" +
				"  \"body\" TEXT\n" +
				")",
		},
		{
			name:    "table with a default value",
			dialect: DialectPostgres,
			schema:  "",
			table:   "accounts",
			columns: []ColumnDefinition{
				{Name: "status", Type: ColumnTypeText, Nullable: false, Default: strPtr("'active'")},
				{Name: "created_at", Type: ColumnTypeTimestamp, Nullable: false, Default: strPtr("now()")},
			},
			want: "CREATE TABLE \"accounts\" (\n" +
				"  \"status\" TEXT NOT NULL DEFAULT 'active',\n" +
				"  \"created_at\" TIMESTAMP NOT NULL DEFAULT now()\n" +
				")",
		},
		{
			name:    "timestamp column resolves to DATETIME on MySQL",
			dialect: DialectMySQL,
			schema:  "",
			table:   "events",
			columns: []ColumnDefinition{
				{Name: "occurred_at", Type: ColumnTypeTimestamp, Nullable: false},
			},
			want: "CREATE TABLE `events` (\n" +
				"  `occurred_at` DATETIME NOT NULL\n" +
				")",
		},
		{
			name:    "integer resolves per dialect (INTEGER vs INT)",
			dialect: DialectMySQL,
			schema:  "",
			table:   "counters",
			columns: []ColumnDefinition{
				{Name: "n", Type: ColumnTypeInteger, Nullable: false},
			},
			want: "CREATE TABLE `counters` (\n" +
				"  `n` INT NOT NULL\n" +
				")",
		},
		{
			name:    "a blank default is treated as no default",
			dialect: DialectPostgres,
			schema:  "",
			table:   "t",
			columns: []ColumnDefinition{
				{Name: "id", Type: ColumnTypeInteger, Nullable: false, Default: strPtr("   ")},
			},
			want: "CREATE TABLE \"t\" (\n  \"id\" INTEGER NOT NULL\n)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildCreateTableDDL(tc.dialect, tc.schema, tc.table, tc.columns)
			if err != nil {
				t.Fatalf("BuildCreateTableDDL() unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("BuildCreateTableDDL() =\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestBuildCreateTableDDL_Errors(t *testing.T) {
	tests := []struct {
		name    string
		dialect Dialect
		table   string
		columns []ColumnDefinition
	}{
		{
			name:    "empty table name",
			dialect: DialectPostgres,
			table:   "",
			columns: []ColumnDefinition{{Name: "id", Type: ColumnTypeInteger}},
		},
		{
			name:    "no columns",
			dialect: DialectPostgres,
			table:   "widgets",
			columns: nil,
		},
		{
			name:    "blank column name",
			dialect: DialectPostgres,
			table:   "widgets",
			columns: []ColumnDefinition{{Name: "  ", Type: ColumnTypeInteger}},
		},
		{
			name:    "duplicate column names",
			dialect: DialectPostgres,
			table:   "widgets",
			columns: []ColumnDefinition{
				{Name: "id", Type: ColumnTypeInteger},
				{Name: "id", Type: ColumnTypeText},
			},
		},
		{
			name:    "unrecognized column type",
			dialect: DialectPostgres,
			table:   "widgets",
			columns: []ColumnDefinition{{Name: "id", Type: ColumnType("uuid")}},
		},
		{
			name:    "serial column not marked as primary key",
			dialect: DialectPostgres,
			table:   "widgets",
			columns: []ColumnDefinition{{Name: "id", Type: ColumnTypeSerial, IsPrimaryKey: false}},
		},
		{
			name:    "bigserial column not marked as primary key, MySQL",
			dialect: DialectMySQL,
			table:   "widgets",
			columns: []ColumnDefinition{{Name: "id", Type: ColumnTypeBigSerial, IsPrimaryKey: false}},
		},
		{
			name:    "serial column with an explicit default",
			dialect: DialectPostgres,
			table:   "widgets",
			columns: []ColumnDefinition{{Name: "id", Type: ColumnTypeSerial, IsPrimaryKey: true, Default: strPtr("1")}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildCreateTableDDL(tc.dialect, "", tc.table, tc.columns)
			if err == nil {
				t.Fatal("BuildCreateTableDDL() expected an error, got nil")
			}
		})
	}
}
