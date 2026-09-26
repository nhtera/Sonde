// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package convert

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

func mustFile(t *testing.T, url string) *syntax.File {
	t.Helper()
	f, err := syntax.BuildFile([]syntax.EntrySpec{{
		Method: "GET",
		URL:    syntax.PlainText(url),
	}}, syntax.DialectHurl)
	if err != nil {
		t.Fatalf("BuildFile: %v", err)
	}
	return f
}

func TestWriteBasic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out")
	out := Output{Files: []GeneratedFile{
		{Path: "pets/list-pets", File: mustFile(t, "https://example.com/pets")},
		{Path: "pets/get-pet", File: mustFile(t, "https://example.com/pets/1")},
	}}
	res, err := Write(target, out, Options{})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if res.RequestCount != 2 {
		t.Errorf("RequestCount = %d, want 2", res.RequestCount)
	}
	wantFiles := []string{"pets/get-pet.hurl", "pets/list-pets.hurl"}
	if !equalStrings(res.Files, wantFiles) {
		t.Errorf("Files = %v, want %v", res.Files, wantFiles)
	}
	for _, f := range wantFiles {
		if _, err := os.Stat(filepath.Join(target, filepath.FromSlash(f))); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
}

func TestWriteExtSonde(t *testing.T) {
	dir := t.TempDir()
	out := Output{Files: []GeneratedFile{{Path: "a", File: mustFile(t, "https://example.com")}}}
	res, err := Write(dir, out, Options{Ext: "sonde"})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if len(res.Files) != 1 || res.Files[0] != "a.sonde" {
		t.Errorf("Files = %v, want [a.sonde]", res.Files)
	}
}

func TestWriteBadExt(t *testing.T) {
	dir := t.TempDir()
	_, err := Write(dir, Output{}, Options{Ext: "bogus"})
	if err == nil {
		t.Fatal("want error for bad --ext")
	}
}

func TestWriteSanitizesAndDedupsPaths(t *testing.T) {
	dir := t.TempDir()
	out := Output{Files: []GeneratedFile{
		{Path: "../etc/passwd", File: mustFile(t, "https://example.com/1")},
		{Path: "Pets List!!", File: mustFile(t, "https://example.com/2")},
		{Path: "Pets List!!", File: mustFile(t, "https://example.com/3")},
		{Path: "", File: mustFile(t, "https://example.com/4")},
	}}
	res, err := Write(dir, out, Options{})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	want := []string{"etc/passwd.hurl", "pets-list.hurl", "pets-list-2.hurl", "request.hurl"}
	got := append([]string(nil), res.Files...)
	if !equalStrings(sortedCopy(got), sortedCopy(want)) {
		t.Errorf("Files = %v, want (any order matching) %v", got, want)
	}
	for _, f := range got {
		if filepath.IsAbs(f) {
			t.Errorf("path %q must not be absolute", f)
		}
	}
	// No file escaped dir.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Errorf("dir has %d entries, want 4: %v", len(entries), entries)
	}
	parent := filepath.Dir(dir)
	if _, err := os.Stat(filepath.Join(parent, "etc", "passwd.hurl")); err == nil {
		t.Fatal("path escaped the output directory")
	}
}

func TestWriteConflictsWithoutForce(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.hurl"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := Output{Files: []GeneratedFile{
		{Path: "a", File: mustFile(t, "https://example.com/1")},
		{Path: "b", File: mustFile(t, "https://example.com/2")},
	}}
	_, err := Write(dir, out, Options{})
	if err == nil {
		t.Fatal("want conflict error")
	}
	var ce *ErrConflicts
	if !asErrConflicts(err, &ce) {
		t.Fatalf("err = %v (%T), want *ErrConflicts", err, err)
	}
	if len(ce.Files) != 1 || ce.Files[0] != "a.hurl" {
		t.Errorf("Files = %v, want [a.hurl]", ce.Files)
	}
	// Nothing was written, including the non-conflicting file.
	if _, err := os.Stat(filepath.Join(dir, "b.hurl")); err == nil {
		t.Error("b.hurl should not have been written")
	}
	// The pre-existing file is untouched.
	got, err := os.ReadFile(filepath.Join(dir, "a.hurl"))
	if err != nil || string(got) != "existing" {
		t.Errorf("a.hurl = %q, %v, want \"existing\"", got, err)
	}
}

func TestWriteForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.hurl"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := Output{Files: []GeneratedFile{{Path: "a", File: mustFile(t, "https://example.com/1")}}}
	if _, err := Write(dir, out, Options{Force: true}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "a.hurl"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte("existing")) {
		t.Errorf("a.hurl was not overwritten: %s", got)
	}
}

func TestWriteDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	out := Output{Files: []GeneratedFile{{Path: "a", File: mustFile(t, "https://example.com")}}}
	res, err := Write(dir, out, Options{DryRun: true})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !res.DryRun || len(res.Files) != 1 {
		t.Errorf("res = %+v", res)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("dry run wrote %d entries: %v", len(entries), entries)
	}
}

func TestWriteProjectYAMLOnlyOnce(t *testing.T) {
	dir := t.TempDir()
	out := Output{
		Files:       []GeneratedFile{{Path: "a", File: mustFile(t, "https://example.com")}},
		ProjectYAML: []byte("version: 1\n"),
	}
	res, err := Write(dir, out, Options{})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if res.Project != "sonde.yaml" || res.ProjectSkipped {
		t.Errorf("res = %+v", res)
	}
	got, err := os.ReadFile(filepath.Join(dir, "sonde.yaml"))
	if err != nil || string(got) != "version: 1\n" {
		t.Fatalf("sonde.yaml = %q, %v", got, err)
	}

	// A second run, even with --force, must not touch the existing
	// sonde.yaml.
	out.ProjectYAML = []byte("version: 1\nextra: true\n")
	res, err = Write(dir, out, Options{Force: true})
	if err != nil {
		t.Fatalf("Write (2nd): %v", err)
	}
	if res.Project != "" || !res.ProjectSkipped {
		t.Errorf("res (2nd) = %+v", res)
	}
	got, err = os.ReadFile(filepath.Join(dir, "sonde.yaml"))
	if err != nil || string(got) != "version: 1\n" {
		t.Fatalf("sonde.yaml was overwritten: %q, %v", got, err)
	}
}

func TestWriteFormatsOutput(t *testing.T) {
	dir := t.TempDir()
	f, err := syntax.Parse("<t>", []byte("GET   https://example.com\n"), syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	out := Output{Files: []GeneratedFile{{Path: "a", File: f}}}
	if _, err := Write(dir, out, Options{}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "a.hurl"))
	if err != nil {
		t.Fatal(err)
	}
	want := syntax.Format(f)
	if !bytes.Equal(got, want) {
		t.Errorf("a.hurl = %q, want Format() output %q", got, want)
	}
}

func TestSanitizeSegment(t *testing.T) {
	tests := map[string]string{
		"Pets List":  "pets-list",
		"a/b":        "a-b",
		"":           "request",
		"---":        "request",
		"CON":        "con-file",
		"a_b.c":      "a-b-c",
		"héllo":      "h-llo",
		"already-ok": "already-ok",
	}
	for in, want := range tests {
		if got := sanitizeSegment(in); got != want {
			t.Errorf("sanitizeSegment(%q) = %q, want %q", in, got, want)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sortedCopy(s []string) []string {
	out := append([]string(nil), s...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

func asErrConflicts(err error, target **ErrConflicts) bool {
	ce, ok := err.(*ErrConflicts)
	if ok {
		*target = ce
	}
	return ok
}
