# RecordLite

RecordLite is a library (and executable) that declaratively maintains SQLite tables
and views of semi-structured data (henceforth known as records). RecordLite is
based on a hidden gem in Backtrace's [sqlite_protobuf](https://github.com/backtrace-labs/sqlite_protobuf/blob/141486b492ccf342cbba6fa40e076a8941afc839/proto_table/proto_table.c)
library.

TLDR: RecordLite stores semi-structured records into SQLite table where one column is
the raw payload (JSON or Protobuf) and define views with virtual columns from the
raw column via extraction functions, e.g. [json_extract](https://www.sqlite.org/json1.html#jex) and
[protobuf_extract](https://github.com/rgov/sqlite_protobuf#protobuf_extractprotobuf-type_name-path).
Tell RecordLite what you want the view and its indexes to look like, and RecordLite
spits out idempotent DDL statements to make that happen, for any initial state and
with minimal churn.

## Installation

To install the `recordlite` executable via `go get`.

```
# go install github.com/fsaintjacques/recordlite/cmd/recordlite@latest
```

To install `recordlite` as a go module dependency

```
# go get github.com/fsaintjacques/recordlite/cmd/recordlite@latest
```

## About

### Why

Schema management is a hard practical problem. By using semi-structured data
and storing it as a single BLOB column, one does not need to modify the "true"
table's schema, only the dynamic views.

### How

For a given table of records, RecordLite generates a companion view that exposes
virtual columns of fields of interested defined by the user. The columns can be
optionally indexed if they're often projected and/or used for filtering. The
column are defined with a function extracting the data from the raw column
(either JSON or Protobuf), note that this would also work for any type of
expression.

Since the view does not own the data, it is safe to delete/add/update columns
without affecting the underlying table. In other words, it is relatively cheap
to modify the view since it will not trigger a massive scan + write loop. OTOH,
updating any index will require recomputing the index.

## Examples

```
$ # Define a schema
$ cat schema.json
{
  "name": "records",
  "columns": [
    {"name": "status", "expr": "json_extract(raw, '$.status')", "with_index": true},
    {"name": "color", "expr": "json_extract(raw, '$.attrs.color')", "with_index": true}
  ]
}

$ # Create the table and views definitions
$ recordlite schema.json | sqlite3 records.db

$ # Simulate a process appending to the records
$ cat << EOF | awk '{print "INSERT INTO records(raw) VALUES('"'"'" $0 "'"'"');"}' | sqlite3 records.db
> {"status":"ok", "attrs": {"color": "red", "size": "big"}}
> {"status":"failed", "attrs": {"color":"blue", "size": "small"}}
> EOF

$ sqlite3 --box records.db
sqlite> SELECT id, status, color from records;
┌────┬────────┬───────┐
│ id │ status │ color │
├────┼────────┼───────┤
│ 1  │ ok     │ red   │
│ 2  │ failed │ blue  │
└────┴────────┴───────┘

$ # Let's modify the schema to index attrs.size
$ cat schema.json
{
  "name": "records",
  "columns": [
    {"name": "status", "expr": "json_extract(raw, '$.status')", "with_index": true},
    {"name": "color", "expr": "json_extract(raw, '$.attrs.color')", "with_index": true},
    {"name": "size", "expr": "json_extract(raw, '$.attrs.size')", "with_index": true}
  ]
}

$ # Update the views definition
$ recordlite schema.json | sqlite3 records.db

$ sqlite3 --box records.db
sqlite> SELECT id, status, color, size FROM records;
┌────┬────────┬───────┬───────┐
│ id │ status │ color │ size  │
├────┼────────┼───────┼───────┤
│ 1  │ ok     │ red   │ big   │
│ 2  │ failed │ blue  │ small │
└────┴────────┴───────┴───────┘

```
