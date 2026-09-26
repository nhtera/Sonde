// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package convert

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWriteExtraFiles(t *testing.T) {
	dir := t.TempDir()
	out := Output{
		Files: []GeneratedFile{{Path: "vars", File: mustFile(t, "https://example.com")}},
		Extra: []RawFile{
			{Path: "vars.env", Data: []byte("a=1\n")},
			{Path: "secrets/prod-env.env", Data: []byte("token=\n"), Keep: true},
		},
		ProjectYAML: []byte("version: 1\n"),
	}
	res, err := Write(dir, out, Options{})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	want := []string{"secrets/prod-env.env", "vars.env"}
	if !equalStrings(res.Extra, want) {
		t.Fatalf("Extra = %v, want %v", res.Extra, want)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "vars.env")); string(got) != "a=1\n" {
		t.Errorf("vars.env = %q", got)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(filepath.Join(dir, "secrets", "prod-env.env"))
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Errorf("kept file mode = %v, want 0600", st.Mode().Perm())
		}
	}

	// A second run: the kept file is left alone even with --force; the
	// others conflict without it.
	if err := os.WriteFile(filepath.Join(dir, "secrets", "prod-env.env"), []byte("token=filled\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var conflicts *ErrConflicts
	if _, err := Write(dir, out, Options{}); !errors.As(err, &conflicts) {
		t.Fatalf("second Write err = %v, want conflicts", err)
	}
	if strings.Contains(strings.Join(conflicts.Files, ","), "prod-env") {
		t.Errorf("kept file reported as a conflict: %v", conflicts.Files)
	}
	res, err = Write(dir, out, Options{Force: true})
	if err != nil {
		t.Fatalf("forced Write: %v", err)
	}
	if !equalStrings(res.ExtraKept, []string{"secrets/prod-env.env"}) {
		t.Errorf("ExtraKept = %v", res.ExtraKept)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "secrets", "prod-env.env")); string(got) != "token=filled\n" {
		t.Errorf("kept file overwritten: %q", got)
	}

	var b bytes.Buffer
	Summarize(&b, out, res)
	for _, s := range []string{"wrote vars.env", "secrets/prod-env.env: already exists, left untouched"} {
		if !strings.Contains(b.String(), s) {
			t.Errorf("summary missing %q:\n%s", s, b.String())
		}
	}
}

func TestWriteExtraDryRunMissingDir(t *testing.T) {
	target := filepath.Join(t.TempDir(), "new")
	out := Output{Extra: []RawFile{{Path: "http-client-env.json", Data: []byte("{}")}}}
	res, err := Write(target, out, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(res.Extra, []string{"http-client-env.json"}) {
		t.Errorf("Extra = %v", res.Extra)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("dry run created %s", target)
	}
}

func TestWriteExtraRejectsNonCanonicalPaths(t *testing.T) {
	for _, p := range []string{"../vars.env", "Secrets/Prod.env", "sonde.yaml", ".env", "secrets/.env", "f.$$", "noext", "a b.env"} {
		out := Output{Extra: []RawFile{{Path: p, Data: []byte("x")}}}
		if _, err := Write(t.TempDir(), out, Options{DryRun: true}); err == nil {
			t.Errorf("Write accepted extra path %q", p)
		}
	}
	dup := Output{Extra: []RawFile{{Path: "a.env"}, {Path: "a.env"}}}
	if _, err := Write(t.TempDir(), dup, Options{DryRun: true}); err == nil {
		t.Error("Write accepted a duplicate extra path")
	}
}

func TestStubPath(t *testing.T) {
	used := map[string]bool{}
	got := []string{StubPath("Prod", used), StubPath("prod!", used), StubPath("My Env", used), StubPath("con", used), StubPath("", used)}
	want := []string{"secrets/prod.secrets", "secrets/prod-2.secrets", "secrets/my-env.secrets", "secrets/con-file.secrets", "secrets/request.secrets"}
	if !equalStrings(got, want) {
		t.Errorf("StubPath = %v, want %v", got, want)
	}
	// Every stub path is accepted by Write as is.
	var out Output
	for _, p := range got {
		out.Extra = append(out.Extra, RawFile{Path: p, Keep: true})
	}
	res, err := Write(t.TempDir(), out, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(res.Extra, sortedCopy(got)) {
		t.Errorf("written %v, want %v", res.Extra, got)
	}
}

func TestSanitizeSegmentLong(t *testing.T) {
	long := strings.Repeat("b", 245)
	cjk := strings.Repeat("日本", 60)
	for _, in := range []string{long, cjk, long + "x"} {
		got := sanitizeSegment(in)
		if len(got) > maxSegment || !utf8.ValidString(got) {
			t.Errorf("sanitizeSegment(%d bytes) = %q (%d bytes)", len(in), got, len(got))
		}
		if sanitizeSegment(got) != got {
			t.Errorf("sanitizeSegment is not idempotent on %q", got)
		}
	}
	if sanitizeSegment(long) == sanitizeSegment(long+"x") {
		t.Error("two long names truncate to the same segment")
	}
	dir := t.TempDir()
	out := Output{Files: []GeneratedFile{{Path: long, File: mustFile(t, "https://example.com")}}}
	if _, err := Write(dir, out, Options{}); err != nil {
		t.Fatalf("Write long name: %v", err)
	}
}
