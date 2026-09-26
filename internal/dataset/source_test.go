// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package dataset

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func readAll(t *testing.T, path string) ([]Row, error) {
	t.Helper()
	var rows []Row
	err := Each(path, func(r Row) error {
		rows = append(rows, r)
		return nil
	})
	return rows, err
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCSV(t *testing.T) {
	rows, err := readAll(t, "../../testdata/dataset/users.csv")
	if err != nil {
		t.Fatal(err)
	}
	want := []Row{
		{Index: 1, Line: 2, Fields: []Field{{Name: "user", Raw: "alice"}, {Name: "password", Raw: "s3cret-1"}, {Name: "note", Raw: `hello, "world"`}}},
		{Index: 2, Line: 3, Fields: []Field{{Name: "user", Raw: "bob"}, {Name: "password", Raw: "s3cret-2"}, {Name: "note", Raw: "multi\nline"}}},
		{Index: 3, Line: 5, Fields: []Field{{Name: "user", Raw: "carol"}, {Name: "password", Raw: ""}, {Name: "note", Raw: ""}}},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows =\n%+v\nwant\n%+v", rows, want)
	}
}

func TestJSON(t *testing.T) {
	rows, err := readAll(t, "../../testdata/dataset/users.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows", len(rows))
	}
	want := []Field{
		{Name: "admin", Raw: "false", JSON: true},
		{Name: "age", Raw: "12345678901234567890", JSON: true},
		{Name: "meta", Raw: `{"k": null}`, JSON: true},
		{Name: "password", Raw: `"s3cret-2"`, JSON: true},
		{Name: "user", Raw: `"bob"`, JSON: true},
	}
	if rows[1].Index != 2 || !reflect.DeepEqual(rows[1].Fields, want) {
		t.Errorf("row 2 = %+v", rows[1])
	}
}

func TestEmptyFiles(t *testing.T) {
	rows, err := readAll(t, writeFile(t, "a.csv", "user,password\n"))
	if err != nil || len(rows) != 0 {
		t.Errorf("header-only CSV: %d rows, %v", len(rows), err)
	}
	rows, err = readAll(t, writeFile(t, "a.json", " [ ] \n"))
	if err != nil || len(rows) != 0 {
		t.Errorf("empty JSON array: %d rows, %v", len(rows), err)
	}
}

func TestErrors(t *testing.T) {
	for _, tc := range []struct {
		name, content, want string
	}{
		{"a.txt", "x", "unsupported data file format"},
		{"a.csv", "", "missing header row"},
		{"a.csv", "user,user\n", `:1: duplicate column "user"`},
		{"a.csv", "user, \n", ":1: column 2 has a blank name"},
		{"a.csv", "a,b\n1,2\n3\n", ":3: wrong number of fields"},
		{"a.csv", "a\n\"open\n", `extraneous or missing " in quoted-field`},
		{"a.csv", "a\n\xff\n", `:2: column "a" is not valid UTF-8`},
		{"a.json", `{"a": 1}`, "expected a JSON array of objects"},
		{"a.json", `[{"a": 1}, 2]`, "row 2: expected an object"},
		{"a.json", `[{"a": 1}, null]`, "row 2: expected an object"},
		{"a.json", `[{"a": 1}, {"a": }]`, "row 2: invalid character"},
		{"a.json", `[{"a": 1}`, "unexpected end of JSON"},
		{"a.json", `[{"a": 1}] [`, "unexpected data after the array"},
		{"a.json", `[{"": 1}]`, "row 1: blank column name"},
	} {
		_, err := readAll(t, writeFile(t, tc.name, tc.content))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s %q: error %v, want %q", tc.name, tc.content, err, tc.want)
		}
	}
}

func TestMissingFile(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "missing.csv")); err == nil {
		t.Error("no error for a missing file")
	}
}

// CSV edge cases
func TestCSVBOM(t *testing.T) {
	content := "\xef\xbb\xbfuser,password\nalice,pass\n"
	rows, err := readAll(t, writeFile(t, "bom.csv", content))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Fields[0].Raw != "alice" {
		t.Errorf("BOM CSV: got %+v, want alice", rows)
	}
}

func TestCSVCRLF(t *testing.T) {
	content := "user,password\r\nalice,pass\r\nbob,secret\r\n"
	rows, err := readAll(t, writeFile(t, "crlf.csv", content))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Fields[0].Raw != "alice" || rows[1].Fields[0].Raw != "bob" {
		t.Errorf("CRLF CSV: got %d rows", len(rows))
	}
}

func TestCSVQuotedHeader(t *testing.T) {
	content := "\"user\",\"password\"\nalice,pass\n"
	rows, err := readAll(t, writeFile(t, "quoted.csv", content))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Fields[0].Name != "user" {
		t.Errorf("Quoted header CSV: got %+v", rows[0])
	}
}

func TestCSVTrailingEmptyLine(t *testing.T) {
	content := "user,password\nalice,pass\nbob,secret\n\n"
	rows, err := readAll(t, writeFile(t, "trailing.csv", content))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Errorf("Trailing empty CSV: got %d rows", len(rows))
	}
}

func TestCSVBOMAndQuotedFirstCell(t *testing.T) {
	content := "\xef\xbb\xbf\"user\",password\n\"alice\",pass\n"
	rows, err := readAll(t, writeFile(t, "bom_quoted.csv", content))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Fields[0].Raw != "alice" {
		t.Errorf("BOM + quoted CSV: got %+v", rows[0])
	}
}

func TestCSVQuotedCellWithCommaAndNewline(t *testing.T) {
	content := "user,note\nalice,\"a,b\"\nbob,\"line1\nline2\"\n"
	rows, err := readAll(t, writeFile(t, "quoted_cells.csv", content))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Errorf("Quoted cells CSV: got %d rows", len(rows))
	}
	if rows[0].Fields[1].Raw != "a,b" {
		t.Errorf("Quoted comma: got %q", rows[0].Fields[1].Raw)
	}
	if rows[1].Fields[1].Raw != "line1\nline2" {
		t.Errorf("Quoted newline: got %q", rows[1].Fields[1].Raw)
	}
}

func TestCSVFieldCountMismatch(t *testing.T) {
	content := "a,b,c\n1,2\n"
	_, err := readAll(t, writeFile(t, "mismatch.csv", content))
	if err == nil || !strings.Contains(err.Error(), "wrong number of fields") {
		t.Errorf("Field mismatch: got error %v", err)
	}
}

// JSON edge cases
func TestJSONNested(t *testing.T) {
	content := `[{"user": "alice", "meta": {"age": 30, "country": "US"}}]`
	rows, err := readAll(t, writeFile(t, "nested.json", content))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("Nested JSON: got %d rows", len(rows))
	}
	var found bool
	for _, f := range rows[0].Fields {
		if f.Name == "meta" && f.JSON && strings.Contains(f.Raw, "age") {
			found = true
		}
	}
	if !found {
		t.Errorf("Nested JSON: meta field not found or not JSON")
	}
}

func TestJSONDuplicateKeys(t *testing.T) {
	content := `[{"user": "alice", "user": "bob"}]`
	rows, err := readAll(t, writeFile(t, "dup.json", content))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("Duplicate keys JSON: got %d rows", len(rows))
	}
}

func TestJSONWhitespaceVariants(t *testing.T) {
	content := `
	[
		{
			"user"  :  "alice"  ,
			"password"  :  "pass"
		}
	]
	`
	rows, err := readAll(t, writeFile(t, "ws.json", content))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("Whitespace JSON: got %d rows", len(rows))
	}
	found := false
	for _, f := range rows[0].Fields {
		if f.Name == "user" && f.Raw == `"alice"` {
			found = true
		}
	}
	if !found {
		t.Errorf("Whitespace JSON: user field not found correctly in %+v", rows[0].Fields)
	}
}

func TestJSONVeryLongString(t *testing.T) {
	longStr := strings.Repeat("a", 10000)
	content := fmt.Sprintf(`[{"user": "alice", "data": "%s"}]`, longStr)
	rows, err := readAll(t, writeFile(t, "long.json", content))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("Long string JSON: got %d rows", len(rows))
	}
}

func TestJSONTopLevelNonArray(t *testing.T) {
	content := `{"user": "alice"}`
	_, err := readAll(t, writeFile(t, "nonarray.json", content))
	if err == nil || !strings.Contains(err.Error(), "expected a JSON array") {
		t.Errorf("Non-array JSON: got error %v", err)
	}
}

func TestJSONArray(t *testing.T) {
	content := `[1, 2, 3]`
	_, err := readAll(t, writeFile(t, "array.json", content))
	if err == nil || !strings.Contains(err.Error(), "expected an object") {
		t.Errorf("Scalar array JSON: got error %v", err)
	}
}

func TestCSVEmptyCell(t *testing.T) {
	content := "user,password,note\nalice,,empty\n"
	rows, err := readAll(t, writeFile(t, "empty.csv", content))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Fields[1].Raw != "" {
		t.Errorf("Empty cell CSV: got %+v", rows[0])
	}
}
