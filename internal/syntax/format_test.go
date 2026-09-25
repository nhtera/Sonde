// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"reflect"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestFormatGolden compares Format output of testdata/syntax/*.hurl with
// the matching .fmt.golden file (go test -update rewrites them).
func TestFormatGolden(t *testing.T) {
	files := localSyntaxFiles(t)
	if len(files) == 0 {
		t.Fatal("no testdata/syntax files")
	}
	for _, path := range files {
		f, err := Parse(path, readFile(t, path), DialectHurl)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		got := Format(f)
		golden := strings.TrimSuffix(path, ".hurl") + ".fmt.golden"
		if *update {
			if err := os.WriteFile(golden, got, 0o644); err != nil { //nolint:gosec // G306: committed fixture, world-readable
				t.Fatal(err)
			}
			continue
		}
		if want := readFile(t, golden); !bytes.Equal(got, want) {
			t.Errorf("%s: Format differs from %s:\n%s", path, golden, got)
		}
	}
}

// dump renders the AST as JSON without trivia (whitespace, line terminators,
// spans), so two files that differ only in layout dump identically.
func dump(t testing.TB, f *File) string {
	t.Helper()
	b, err := json.MarshalIndent(strip(reflect.ValueOf(f)), "", " ")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var (
	whitespaceType = reflect.TypeFor[Whitespace]()
	spanType       = reflect.TypeFor[Span]()
	ltType         = reflect.TypeFor[*LineTerminator]()
	ltsType        = reflect.TypeFor[[]*LineTerminator]()
)

func strip(v reflect.Value) any {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		inner := strip(v.Elem())
		if v.Kind() == reflect.Interface {
			return map[string]any{v.Elem().Type().String(): inner}
		}
		return inner
	case reflect.Struct:
		m := map[string]any{}
		for i := range v.NumField() {
			f := v.Type().Field(i)
			switch {
			case !f.IsExported(), f.Type == whitespaceType, f.Type == spanType, f.Type == ltType,
				f.Type == ltsType, f.Type.Kind() == reflect.String && strings.HasPrefix(f.Name, "Space"):
				continue
			}
			m[f.Name] = strip(v.Field(i))
		}
		return m
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return v.Bytes()
		}
		out := make([]any, v.Len())
		for i := range v.Len() {
			out[i] = strip(v.Index(i))
		}
		return out
	default:
		return v.Interface()
	}
}

func TestFormatConformanceFiles(t *testing.T) {
	files := append(conformanceFiles(t), localSyntaxFiles(t)...)
	for _, path := range files {
		if _, fail := expectedFailures[conformanceName(path)]; fail {
			continue
		}
		f, err := Parse(path, readFile(t, path), DialectHurl)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		checkFormat(t, path, f)
	}
}

// TestFormatKeepsLintedFiles: the linted conformance suites are already in
// canonical layout, so Format leaves them unchanged.
func TestFormatKeepsLintedFiles(t *testing.T) {
	for _, path := range conformanceFiles(t) {
		name := conformanceName(path)
		if !strings.HasPrefix(name, "tests_ok/") && !strings.HasPrefix(name, "tests_failed/") {
			continue
		}
		src := readFile(t, path)
		f, err := Parse(name, src, DialectHurl)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := Format(f); !bytes.Equal(got, src) {
			t.Errorf("%s: Format changed a linted file at byte %d", name, firstDiff(got, src))
		}
	}
}

func checkFormat(t *testing.T, name string, f *File) {
	t.Helper()
	formatted := Format(f)
	g, err := Parse(name, formatted, DialectHurl)
	if err != nil {
		t.Errorf("%s: formatted output does not parse: %v\n%s", name, err, formatted)
		return
	}
	if again := Format(g); !bytes.Equal(again, formatted) {
		t.Errorf("%s: Format is not idempotent (first difference at byte %d)", name, firstDiff(again, formatted))
	}
	if dump(t, f) != dump(t, g) {
		t.Errorf("%s: formatting changed the meaning of the file", name)
	}
}

func TestFormatRules(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"spacing", "  GET   http://a  \n  Accept :  */*\n", "GET http://a\nAccept: */*\n"},
		{"final newline", "GET http://a", "GET http://a\n"},
		{"blank lines kept", "GET http://a\nGET http://b\n\n\nGET http://c\n", "GET http://a\nGET http://b\n\n\nGET http://c\n"},
		{"blank lines emptied", "\n  \n# top\nGET http://a\n \t\n", "\n\n# top\nGET http://a\n\n"},
		{"comments kept", "GET http://a   # trailing  \n", "GET http://a   # trailing\n"},
		{"crlf", "GET http://a\r\nHTTP 200\r\n", "GET http://a\nHTTP 200\n"},
		{"asserts", "GET http://a\nHTTP  200\n[Asserts]\n  jsonpath   \"$.a\"   count   not   ==2\n  header \"x\"   exists\n",
			"GET http://a\nHTTP 200\n[Asserts]\njsonpath \"$.a\" count not == 2\nheader \"x\" exists\n"},
		{"captures", "GET http://a\nHTTP 200\n[Captures]\n id :jsonpath \"$.id\"   redact\n",
			"GET http://a\nHTTP 200\n[Captures]\nid: jsonpath \"$.id\" redact\n"},
		{"options", "GET http://a\n[Options]\n  delay :  2s\n variable: a = 1\n",
			"GET http://a\n[Options]\ndelay: 2s\nvariable: a=1\n"},
		{"empty header value", "GET http://a\nX-Empty:\n", "GET http://a\nX-Empty:\n"},
		{"body untouched", "POST http://a\n  {\n    \"a\" :  1\n}\n", "POST http://a\n{\n    \"a\" :  1\n}\n"},
		{"multipart", "POST http://a\n[Multipart]\n f:  file, a.txt ;  text/plain\n",
			"POST http://a\n[Multipart]\nf: file,a.txt; text/plain\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse("t.hurl", []byte(tt.in), DialectHurl)
			if err != nil {
				t.Fatal(err)
			}
			if got := string(Format(f)); got != tt.want {
				t.Errorf("Format:\n%q\nwant:\n%q", got, tt.want)
			}
			checkFormat(t, tt.name, f)
		})
	}
}
