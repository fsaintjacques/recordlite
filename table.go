package recordlite

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"strings"
	"text/template"
)

type (
	ColumnDef struct {
		// Name of the column to use.
		Name string
		// Expression of the column.
		Expr string
		// Indicates if the column's expression should be indexed in the raw table.
		// The expression's string is hashed to avoid re-creating indices. This has
		// the side effect that simple aesthetic change to the expression will
		// trigger a full index rebuild for said column.
		WithIndex bool `json:"with_index"`
	}

	ViewDef struct {
		// Name of the view. The raw sources's table will be named `${Name}_raw`.
		Name    string
		Columns []ColumnDef
		// If enabled, the statements for the view's triggers will not be generated.
		SkipTriggers bool `json:"skip_triggers"`
		// If enabled, the statements for the columns' index will not be generated.
		SkipIndices bool `json:"skip_indices"`
		// If enabled, indices not defined by the columns will be dropped.
		UnsafeDropOrphanIndices bool `json:"unsafe_drop_orphan_indices"`
	}
)

// CompileViewDef returns an SQL statement
func CompileViewDef(def *ViewDef) (string, error) {
	buf := bytes.NewBufferString("")
	if err := root.Execute(buf, def); err != nil {
		return "", fmt.Errorf("pbql: failed executing template: %w", err)
	}

	return strings.Trim(buf.String(), "\n"), nil
}

func (t *ViewDef) Table() string {
	return fmt.Sprintf("%s_raw", t.Name)
}

func (t *ViewDef) View() string {
	return t.Name
}

func (t *ViewDef) IndexNames() string {
	var names []string
	for _, col := range t.Columns {
		if col.WithIndex {
			names = append(names, fmt.Sprintf("'%s'", col.IndexName()))
		}
	}
	return strings.Join(names, ",\n    ")
}

func (c *ColumnDef) SelectExpr(last bool) string {
	comma := ","
	if last {
		comma = ""
	}
	return fmt.Sprintf("%s AS %s%s\n", c.Expr, c.Name, comma)
}

const indexPrefix = "_col_expr"

func (c *ColumnDef) IndexName() string {
	return fmt.Sprintf("%s_%s_%x", indexPrefix, c.Name, sha1.Sum([]byte(c.Expr)))
}

func (c *ColumnDef) CreateIndexStatement(table string) string {
	return fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s\n  ON %s(%s);\n\n", c.IndexName(), table, c.Expr)
}

var funcs = map[string]interface{}{
	// A tiny function helper that allows handling commas.
	"last":        func(index, size int) bool { return (index + 1) == size },
	"indexPrefix": func() string { return indexPrefix },
}

func mustParse(tpl *template.Template, payload string) *template.Template {
	tpl, err := tpl.Parse(strings.TrimSpace(payload))
	if err != nil {
		panic(fmt.Sprintf("failed parsing: %s", err.Error()))
	}
	return tpl
}

var (
	root             = mustParse(template.New("root"), rootTemplateStr).Funcs(funcs)
	tableTemplate    = mustParse(root.New("table"), tableTemplateStr)
	viewTemplate     = mustParse(root.New("view"), viewTemplateStr)
	triggersTemplate = mustParse(root.New("triggers"), triggersTemplateStr)
	indicesTemplate  = mustParse(root.New("indices"), indicesTemplateStr)
)

// Global definitions used by sub-templates.
var rootTemplateStr = `
BEGIN EXCLUSIVE;

{{template "table" .}}

--
-- View
--

{{template "view" .}}

{{if not .SkipTriggers -}}
--
-- Triggers
--

-- The trigger helpers enable applications to write in the view and avoid
-- the writing in the raw table. The restriction is that they can only reference
-- the 'raw' column.

{{template "triggers" .}}

{{end -}}
{{if not .SkipIndices -}}
--
-- Indices
--

{{template "indices" . }}
{{end -}}

COMMIT;
`

var tableTemplateStr = `
CREATE TABLE IF NOT EXISTS {{.Table}} (
  id INTEGER PRIMARY KEY NOT NULL,
  raw BLOB NOT NULL
);
`

var viewTemplateStr = `
{{ $nCols := (.Columns | len) -}}
DROP VIEW IF EXISTS {{.View}};
CREATE VIEW IF NOT EXISTS {{.View}} AS
SELECT
  id,
  raw {{- if .Columns}},{{end}}
{{range $index, $val := .Columns}}  {{last $index $nCols | .SelectExpr}}{{end -}}
FROM {{.Table}};`

var triggersTemplateStr = `
DROP TRIGGER IF EXISTS {{.View}}_insert;
CREATE TRIGGER IF NOT EXISTS {{.View}}_insert INSTEAD OF INSERT ON {{.View}}
BEGIN
  INSERT INTO {{.Table}}(raw) VALUES(NEW.raw);
END;

DROP TRIGGER IF EXISTS {{.View}}_update;
CREATE TRIGGER IF NOT EXISTS {{.View}}_update INSTEAD OF UPDATE ON {{.View}}
BEGIN
  UPDATE {{.Table}} SET raw = NEW.raw WHERE id = OLD.id;
END;

DROP TRIGGER IF EXISTS {{.View}}_delete;
CREATE TRIGGER IF NOT EXISTS {{.View}}_delete INSTEAD OF DELETE ON {{.View}}
BEGIN
  DELETE FROM {{.Table}} WHERE id = OLD.id;
END;
`

var indicesTemplateStr = `
{{$table := .Table }}{{$unsafe_drop := .UnsafeDropOrphanIndices}}
{{- range .Columns }}{{if .WithIndex}}{{$table | .CreateIndexStatement}}{{end}}{{end}}
{{- if $unsafe_drop -}}
-- Delete all indices that were previously defined by us, but are not included
-- in the lasted definition.
PRAGMA writable_schema = 1;
DELETE FROM sqlite_master
  WHERE type = 'index' AND tbl_name = '{{.Table}}' AND
  (name LIKE '{{indexPrefix}}_%' AND name NOT IN ({{.IndexNames}}));
PRAGMA writable_schema = 0;
{{end}}
`
