// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package opencollection

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

var update = flag.Bool("update", false, "rewrites the golden files")

// dump renders out deterministically: every generated file's formatted
// source, the extra files, the sonde.yaml skeleton, and the warnings and
// skipped items, sorted. Every generated file must also parse (BuildFile
// already guarantees this; dump reparses defensively so a golden failure
// this test would otherwise miss is caught here too).
func dump(t *testing.T, out convert.Output, dialect syntax.Dialect) string {
	t.Helper()
	var b strings.Builder

	files := append([]convert.GeneratedFile(nil), out.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, f := range files {
		src := syntax.Format(f.File)
		if _, err := syntax.Parse(f.Path, src, dialect); err != nil {
			t.Errorf("%s does not parse: %v", f.Path, err)
		}
		fmt.Fprintf(&b, "== file %s\n%s", f.Path, src)
	}

	extra := append([]convert.RawFile(nil), out.Extra...)
	sort.Slice(extra, func(i, j int) bool { return extra[i].Path < extra[j].Path })
	for _, e := range extra {
		fmt.Fprintf(&b, "== extra %s (keep=%v)\n%s", e.Path, e.Keep, e.Data)
	}

	if out.ProjectYAML != nil {
		fmt.Fprintf(&b, "== sonde.yaml\n%s", out.ProjectYAML)
	}

	warns := append([]convert.Warning(nil), out.Warnings...)
	sort.Slice(warns, func(i, j int) bool {
		if warns[i].Kind != warns[j].Kind {
			return warns[i].Kind < warns[j].Kind
		}
		return warns[i].Message < warns[j].Message
	})
	for _, w := range warns {
		fmt.Fprintf(&b, "warning %s: %s\n", w.Kind, w.Message)
	}

	skipped := append([]convert.Skipped(nil), out.Skipped...)
	sort.Slice(skipped, func(i, j int) bool { return skipped[i].Name < skipped[j].Name })
	for _, s := range skipped {
		fmt.Fprintf(&b, "skipped %s: %s\n", s.Name, s.Reason)
	}
	return b.String()
}

func checkGolden(t *testing.T, got, goldenPath string) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenPath) //nolint:gosec // G304: test fixture
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("output differs from %s (go test -run %s -update):\n%s", goldenPath, t.Name(), got)
	}
}

func TestImportFileGolden(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/convert/opencollection/single-file/collection.yml")
	if err != nil {
		t.Fatal(err)
	}
	out, err := ImportFile(data, syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	checkGolden(t, dump(t, out, syntax.DialectHurl), "../../../testdata/convert/opencollection/single-file.golden")
}

func TestImportDirGolden(t *testing.T) {
	out, err := ImportDir("../../../testdata/convert/opencollection/directory", syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	checkGolden(t, dump(t, out, syntax.DialectHurl), "../../../testdata/convert/opencollection/directory.golden")
}

func TestImportFileVersionWarning(t *testing.T) {
	out, err := ImportFile([]byte("opencollection: \"2.0.0\"\nitems: []\n"), syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range out.Warnings {
		if strings.Contains(w.Message, `unrecognized opencollection version "2.0.0"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("no version warning: %+v", out.Warnings)
	}
}

func TestImportFileMultiDocumentRejected(t *testing.T) {
	_, err := ImportFile([]byte("opencollection: \"1.0.0\"\nitems: []\n---\nfoo: bar\n"), syntax.DialectHurl)
	if err == nil || !strings.Contains(err.Error(), "more than one YAML document") {
		t.Errorf("err = %v", err)
	}
}

func TestImportFileMalformedNeverPanics(_ *testing.T) {
	for _, in := range []string{
		"",
		"not yaml: [",
		"opencollection: 1\nitems:\n  - info: 5\n",
		"items:\n  - info: {name: x, type: http}\n    http: not-a-mapping\n",
	} {
		if _, err := ImportFile([]byte(in), syntax.DialectHurl); err == nil {
			// Some malformed input still parses to an empty/near-empty
			// document (e.g. "" or a document missing "opencollection");
			// that is fine, as long as nothing panicked.
			continue
		}
	}
}

func TestUnsupportedItemsSkipped(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/convert/opencollection/single-file/collection.yml")
	if err != nil {
		t.Fatal(err)
	}
	out, err := ImportFile(data, syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, s := range out.Skipped {
		if strings.Contains(s.Name, "Legacy stream") && strings.Contains(s.Reason, "websocket") {
			found = true
		}
	}
	if !found {
		t.Errorf("websocket item not skipped: %+v", out.Skipped)
	}
}

func TestScriptsNeverExecuted(t *testing.T) {
	// A script's code is kept as a comment only; nothing in this package
	// ever evaluates it (there is no JS engine here to evaluate it with).
	data := []byte(`opencollection: "1.0.0"
items:
  - info: {name: Danger, type: http}
    http: {method: GET, url: "https://example.com"}
    runtime:
      scripts:
        - type: after-response
          code: "require('fs').writeFileSync('pwned', 'x')"
`)
	out, err := ImportFile(data, syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("files = %d", len(out.Files))
	}
	src := string(syntax.Format(out.Files[0].File))
	if !strings.Contains(src, "# opencollection after-response script:") || !strings.Contains(src, "writeFileSync") {
		t.Errorf("script not kept as a comment:\n%s", src)
	}
	found := false
	for _, w := range out.Warnings {
		if w.Kind == convert.WarnScript {
			found = true
		}
	}
	if !found {
		t.Error("no WarnScript warning")
	}
}

func TestSecretNeverWritesValue(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/convert/opencollection/single-file/collection.yml")
	if err != nil {
		t.Fatal(err)
	}
	out, err := ImportFile(data, syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range out.Extra {
		if !strings.Contains(e.Path, "secrets/") {
			continue
		}
		if !e.Keep {
			t.Errorf("%s: secrets stub not marked Keep", e.Path)
		}
		if strings.Contains(string(e.Data), "=") && !strings.HasSuffix(strings.TrimSpace(string(e.Data)), "=") &&
			strings.ContainsAny(string(e.Data), "0123456789") {
			t.Errorf("%s: looks like it holds a value: %q", e.Path, e.Data)
		}
	}
}

// TestSelfReferentialAliasNeverPanics is a regression test: a per-item
// isolated decode (decodeItemNode/decodeItemsFrom, decode.go) must not
// let a self-referential YAML anchor inside "items" recurse without
// bound, since yaml.Node.Decode resets go-yaml's own alias-ratio guard on
// every call. This exact input once overflowed the goroutine stack.
func TestSelfReferentialAliasNeverPanics(t *testing.T) {
	data := []byte("items:\n  - &a {info: {name: x, type: folder}, items: [*a]}\n")
	out, err := ImportFile(data, syntax.DialectHurl)
	if err != nil {
		t.Fatalf("ImportFile: %v", err)
	}
	if len(out.Files) != 0 {
		t.Errorf("files = %d, want 0 (a folder-only cycle has no requests)", len(out.Files))
	}
	found := false
	for _, w := range out.Warnings {
		if strings.Contains(w.Message, "nesting exceeds") {
			found = true
		}
	}
	if !found {
		t.Errorf("no nesting-depth warning: %+v", out.Warnings)
	}
}

// TestPathParamWholeSegmentOnly is a regression test for a path-param
// rewrite that used to replace a bare substring: ":user" must not also
// rewrite half of ":userId".
func TestPathParamWholeSegmentOnly(t *testing.T) {
	data := []byte(`items:
  - info: {name: Get, type: http}
    http:
      method: GET
      url: "/users/:user/:userId"
      params:
        - {name: user, value: "1", type: path}
        - {name: userId, value: "2", type: path}
`)
	out, err := ImportFile(data, syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("files = %d", len(out.Files))
	}
	src := string(syntax.Format(out.Files[0].File))
	if !strings.Contains(src, "/users/{{user}}/{{userId}}") {
		t.Errorf("path params not rewritten as whole segments:\n%s", src)
	}
}

// TestEnvironmentSeededWithDefaults checks M7: a named environment is
// seeded with the collection/default layer's variables (and secret
// names), the environment's own values winning on a name collision.
func TestEnvironmentSeededWithDefaults(t *testing.T) {
	data := []byte(`request:
  variables:
    - {name: base_url, value: "https://default.example.com"}
    - {name: shared, secret: true}
config:
  environments:
    - name: Prod
      variables:
        - {name: base_url, value: "https://prod.example.com"}
items: []
`)
	out, err := ImportFile(data, syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out.ProjectYAML), "base_url: https://prod.example.com") {
		t.Errorf("Prod environment missing its own base_url override:\n%s", out.ProjectYAML)
	}
	if !strings.Contains(string(out.ProjectYAML), "secrets/prod.secrets") {
		t.Errorf("Prod environment not seeded with the default layer's secret %q:\n%s", "shared", out.ProjectYAML)
	}
}

// TestDefaultEnvironmentNameReserved checks M8: an OpenCollection
// environment literally named "default" does not overwrite the
// generated "default" environment.
func TestDefaultEnvironmentNameReserved(t *testing.T) {
	data := []byte(`request:
  variables:
    - {name: base_url, value: "https://collection.example.com"}
config:
  environments:
    - name: default
      variables:
        - {name: base_url, value: "https://named.example.com"}
items: []
`)
	out, err := ImportFile(data, syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out.ProjectYAML), "base_url: https://collection.example.com") {
		t.Errorf("generated default environment overwritten:\n%s", out.ProjectYAML)
	}
	if !strings.Contains(string(out.ProjectYAML), "https://named.example.com") {
		t.Errorf("the renamed environment's own value is missing:\n%s", out.ProjectYAML)
	}
	found := false
	for _, w := range out.Warnings {
		if strings.Contains(w.Message, "renamed to") {
			found = true
		}
	}
	if !found {
		t.Errorf("no rename warning: %+v", out.Warnings)
	}
}

// TestMalformedItemSkippedNotWholeDocument checks M9: a single-file
// document where one item has a field of a YAML type its Go struct
// cannot decode at all (http.method as a mapping) still imports its
// well-formed siblings, with a warning naming the dropped item.
func TestMalformedItemSkippedNotWholeDocument(t *testing.T) {
	data := []byte(`items:
  - info: {name: Bad, type: http}
    http: {method: {x: 1}, url: "https://example.com/bad"}
  - info: {name: Good, type: http}
    http: {method: GET, url: "https://example.com/good"}
`)
	out, err := ImportFile(data, syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 1 || out.Files[0].Path != "Good" {
		t.Fatalf("files = %+v, want just Good", out.Files)
	}
	found := false
	for _, w := range out.Warnings {
		if w.Kind == convert.WarnUnsupported && strings.Contains(w.Message, "item skipped") {
			found = true
		}
	}
	if !found {
		t.Errorf("no item-skipped warning: %+v", out.Warnings)
	}
}

// TestInvalidMethodSkipped checks M3: a method syntax.BuildFile rejects
// (not a valid HTTP token) skips just that request rather than crashing
// or corrupting a sibling entry.
func TestInvalidMethodSkipped(t *testing.T) {
	data := []byte("items:\n  - info: {name: Evil, type: http}\n    http: {method: \"GET evil\\nGET\", url: \"https://example.com\"}\n")
	out, err := ImportFile(data, syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 0 {
		t.Fatalf("files = %+v, want none", out.Files)
	}
	if len(out.Skipped) != 1 {
		t.Fatalf("skipped = %+v, want exactly one", out.Skipped)
	}
}

// TestFileBodyNoSpuriousWarning checks L4: a "file" body does not warn
// "body type file has no Sonde equivalent" — it is supported.
func TestFileBodyNoSpuriousWarning(t *testing.T) {
	data := []byte(`items:
  - info: {name: Upload, type: http}
    http:
      method: POST
      url: "https://example.com/upload"
      body:
        type: file
        data:
          - {filePath: "./a.bin", selected: true}
`)
	out, err := ImportFile(data, syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range out.Warnings {
		if strings.Contains(w.Message, "has no Sonde equivalent") && strings.Contains(w.Message, "file") {
			t.Errorf("spurious warning: %s", w.Message)
		}
	}
}

// TestAuthNoneNoWarning checks L4: auth.type "none" means explicitly no
// auth, not an unsupported scheme.
func TestAuthNoneNoWarning(t *testing.T) {
	data := []byte(`request:
  auth: {type: bearer, token: "{{token}}"}
items:
  - info: {name: Public, type: http}
    http:
      method: GET
      url: "https://example.com"
      auth: {type: none}
`)
	out, err := ImportFile(data, syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("files = %d", len(out.Files))
	}
	src := string(syntax.Format(out.Files[0].File))
	if strings.Contains(src, "Authorization") {
		t.Errorf("auth: none still inherited the collection's bearer auth:\n%s", src)
	}
	for _, w := range out.Warnings {
		if w.Kind == convert.WarnUnsupportedAuth {
			t.Errorf("unexpected auth warning for \"none\": %s", w.Message)
		}
	}
}
