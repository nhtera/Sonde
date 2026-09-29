// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package datarow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/dataset"
)

func writeDataFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSecretTextCSV(t *testing.T) {
	for _, tc := range []struct {
		name     string
		raw      string
		wantText string
		wantOK   bool
	}{
		{"csv string", "mysecret", "mysecret", true},
		{"csv empty", "", "", true},
		{"csv number", "42", "42", true},
		{"csv float", "3.14", "3.14", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := dataset.Field{Name: "secret", Raw: tc.raw, JSON: false}
			s, ok, err := secretText(f)
			if err != nil {
				t.Fatalf("secretText: %v", err)
			}
			if ok != tc.wantOK || s != tc.wantText {
				t.Errorf("secretText: got (%q, %v), want (%q, %v)", s, ok, tc.wantText, tc.wantOK)
			}
		})
	}
}

func TestSecretTextJSON(t *testing.T) {
	for _, tc := range []struct {
		name     string
		raw      string
		wantText string
		wantOK   bool
		wantErr  bool
	}{
		{"json string", `"mysecret"`, "mysecret", true, false},
		{"json number", `42`, "42", true, false},
		{"json float", `3.14`, "3.14", true, false},
		{"json true", `true`, "true", true, false},
		{"json false", `false`, "false", true, false},
		{"json null", `null`, "", false, false},
		{"json whitespace null", `  null  `, "", false, false},
		{"json object", `{"k": 1}`, "", false, true},
		{"json array", `[1, 2]`, "", false, true},
		{"json escaped string", `"a\"b"`, `a"b`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := dataset.Field{Name: "secret", Raw: tc.raw, JSON: true}
			s, ok, err := secretText(f)
			if (err != nil) != tc.wantErr {
				t.Errorf("secretText error: got %v, want err=%v", err, tc.wantErr)
			}
			if !tc.wantErr && (ok != tc.wantOK || s != tc.wantText) {
				t.Errorf("secretText: got (%q, %v), want (%q, %v)", s, ok, tc.wantText, tc.wantOK)
			}
		})
	}
}

func TestNewDataRunValidation(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		secretCols    []string
		wantErr       string
	}{
		{"data_row reserved", "data_row,value\n1,test\n", nil, "reserved"},
		{"data_row as secret", "a,data_row\nb,1\n", []string{"data_row"}, "reserved"},
		{"blank secret col", "a\nb\n", []string{" "}, "blank"},
		{"missing secret col", "a,b\nc,d\n", []string{"missing"}, "no column"},
		{"dup in data", "a,a\nb,c\n", nil, "duplicate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := writeDataFile(t, "rows.csv", tc.content)
			_, err := Open(data, tc.secretCols, nil, nil)
			if err == nil {
				t.Fatalf("Open: expected error with %q", tc.wantErr)
			}
		})
	}
}

func TestNewDataRunEmpty(t *testing.T) {
	data := writeDataFile(t, "rows.csv", "a,b\n")
	d, err := Open(data, nil, nil, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if d.rows != 0 {
		t.Errorf("empty data: got %d rows", d.rows)
	}
}

func TestNewDataRunNoPath(t *testing.T) {
	d, err := Open("", nil, nil, nil)
	if err != nil {
		t.Fatalf("Open empty path: %v", err)
	}
	if d != nil {
		t.Errorf("Open empty path: expected nil")
	}
}

func TestNewDataRunSecretWithoutData(t *testing.T) {
	_, err := Open("", []string{"secret"}, nil, nil)
	if err == nil || err.Error() != "--data-secret requires --data" {
		t.Errorf("--data-secret without --data: got error %v", err)
	}
}

func TestNewDataRunJSONSecret(t *testing.T) {
	data := writeDataFile(t, "rows.json", `[{"secret": {"k": 1}}]`)
	_, err := Open(data, []string{"secret"}, nil, nil)
	if err == nil || err.Error() == "" {
		t.Fatalf("JSON object as secret: expected error")
	}
}

func TestDataRowOverride(t *testing.T) {
	data := writeDataFile(t, "rows.csv", "a,b\n1,2\n")
	d, err := Open(data, nil, []string{"data_row"}, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	row, err := d.row(dataset.Row{Index: 1, Fields: []dataset.Field{
		{Name: "a", Raw: "1"},
		{Name: "b", Raw: "2"},
	}})
	if err != nil {
		t.Fatalf("row conversion: %v", err)
	}
	if _, ok := row.Variables["data_row"]; ok {
		t.Errorf("data_row should be overridden")
	}
}

func TestRowVariableTyping(t *testing.T) {
	data := writeDataFile(t, "rows.csv", "a,b,c\n42,true,hello\n")
	d, err := Open(data, nil, nil, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	row, err := d.row(dataset.Row{Index: 1, Fields: []dataset.Field{
		{Name: "a", Raw: "42", JSON: false},
		{Name: "b", Raw: "true", JSON: false},
		{Name: "c", Raw: "hello", JSON: false},
	}})
	if err != nil {
		t.Fatalf("row conversion: %v", err)
	}
	// Values are wrapped in the value package types, just check they exist
	if _, ok := row.Variables["a"]; !ok {
		t.Errorf("a not in variables")
	}
	if _, ok := row.Variables["b"]; !ok {
		t.Errorf("b not in variables")
	}
	if _, ok := row.Variables["c"]; !ok {
		t.Errorf("c not in variables")
	}
}

func TestRowJSONTyping(t *testing.T) {
	data := writeDataFile(t, "rows.json", `[{"obj": {"k": 1}, "arr": [1, 2], "num": 42}]`)
	d, err := Open(data, nil, nil, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	row, err := d.row(dataset.Row{Index: 1, Fields: []dataset.Field{
		{Name: "obj", Raw: `{"k": 1}`, JSON: true},
		{Name: "arr", Raw: `[1, 2]`, JSON: true},
		{Name: "num", Raw: `42`, JSON: true},
	}})
	if err != nil {
		t.Fatalf("row conversion: %v", err)
	}
	// JSON values are wrapped in value package types
	if _, ok := row.Variables["obj"]; !ok {
		t.Errorf("obj not in variables")
	}
	if _, ok := row.Variables["arr"]; !ok {
		t.Errorf("arr not in variables")
	}
	if _, ok := row.Variables["num"]; !ok {
		t.Errorf("num not in variables")
	}
}

func TestRowSecretsRegistration(t *testing.T) {
	data := writeDataFile(t, "rows.csv", "user,password\nalice,secret123\n")
	d, err := Open(data, []string{"password"}, nil, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	row, err := d.row(dataset.Row{Index: 1, Fields: []dataset.Field{
		{Name: "user", Raw: "alice"},
		{Name: "password", Raw: "secret123"},
	}})
	if err != nil {
		t.Fatalf("row conversion: %v", err)
	}
	if row.Secrets == nil || row.Secrets["password"] != "secret123" {
		t.Errorf("password not registered as secret: %v", row.Secrets)
	}
	if _, ok := row.Variables["password"]; ok {
		t.Errorf("password should not be in variables")
	}
}

func TestEachIteratesAllRows(t *testing.T) {
	data := writeDataFile(t, "rows.csv", "a\n1\n2\n3\n")
	d, err := Open(data, nil, nil, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var count int
	stopped, err := d.Each(func(_ *engine.Row) bool {
		count++
		return true
	})
	if err != nil || stopped {
		t.Errorf("each failed: stopped=%v, err=%v", stopped, err)
	}
	if count != 3 {
		t.Errorf("each: got %d rows, want 3", count)
	}
}

func TestEachStop(t *testing.T) {
	data := writeDataFile(t, "rows.csv", "a\n1\n2\n3\n")
	d, err := Open(data, nil, nil, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var count int
	stopped, err := d.Each(func(_ *engine.Row) bool {
		count++
		return count < 2
	})
	if err != nil || !stopped {
		t.Errorf("each stop: stopped=%v, err=%v", stopped, err)
	}
	if count != 2 {
		t.Errorf("each stop: got %d rows, want 2", count)
	}
}

func TestRowPlaceCSV(t *testing.T) {
	place := rowPlace("data.csv", dataset.Row{Index: 5, Line: 10})
	if place != "data.csv:10" {
		t.Errorf("CSV rowPlace: got %q", place)
	}
}

func TestRowPlaceJSON(t *testing.T) {
	place := rowPlace("data.json", dataset.Row{Index: 5})
	if place != "data.json: row 5" {
		t.Errorf("JSON rowPlace: got %q", place)
	}
}

func TestInvalidJSONSecret(t *testing.T) {
	data := writeDataFile(t, "rows.json", `[{"secret": {}}]`)
	_, err := Open(data, []string{"secret"}, nil, nil)
	if err == nil {
		t.Fatalf("Open should reject object as secret")
	}
}

func TestRowVariableOverride(t *testing.T) {
	data := writeDataFile(t, "rows.csv", "user,pass\nalice,secret\n")
	d, err := Open(data, nil, []string{"pass"}, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	row, err := d.row(dataset.Row{Index: 1, Fields: []dataset.Field{
		{Name: "user", Raw: "alice"},
		{Name: "pass", Raw: "secret"},
	}})
	if err != nil {
		t.Fatalf("row conversion: %v", err)
	}
	if v, ok := row.Variables["pass"]; ok && v != "secret" {
		t.Errorf("pass should not be overridden in variables")
	}
}

func TestSecretColumnConflict(t *testing.T) {
	data := writeDataFile(t, "rows.csv", "user,token\nalice,abc123\n")
	_, err := Open(data, nil, nil, map[string]string{"token": "secret"})
	if err == nil || err.Error() == "" {
		t.Errorf("expected error for secret column conflict")
	}
}

func TestJSONNullSecret(t *testing.T) {
	f := dataset.Field{Name: "secret", Raw: "null", JSON: true}
	s, ok, err := secretText(f)
	if err != nil || ok || s != "" {
		t.Errorf("JSON null secret: got (%q, %v, %v), want (\"\", false, nil)", s, ok, err)
	}
}

func TestJSONStringWithEscapes(t *testing.T) {
	f := dataset.Field{Name: "secret", Raw: `"hello\"world\nnewline"`, JSON: true}
	s, ok, err := secretText(f)
	if err != nil {
		t.Fatalf("secretText: %v", err)
	}
	var expected string
	if err := json.Unmarshal([]byte(f.Raw), &expected); err != nil {
		t.Fatalf("JSON unmarshal: %v", err)
	}
	if s != expected || !ok {
		t.Errorf("JSON escape secret: got (%q, %v), want (%q, true)", s, ok, expected)
	}
}
