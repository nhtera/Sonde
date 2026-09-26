// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// registerFakeImportKind registers a test-only "faketestkind" import kind
// that echoes its INPUT argument as a single GET request, once per test
// binary run (registerImportKind panics on a duplicate name, and Go test
// functions in this package may run more than once under -count).
var registerFakeKindOnce sync.Once

const fakeKindName = "faketestkind"

func registerFakeImportKind(t *testing.T) {
	t.Helper()
	registerFakeKindOnce.Do(func() {
		registerImportKind(importKind{
			Name:  fakeKindName,
			Short: "test-only fake importer",
			RegisterFlags: func(cmd *cobra.Command) {
				cmd.Flags().Bool("fail", false, "make Run fail, for testing")
			},
			Run: func(cmd *cobra.Command, input string, _ convert.Options) (convert.Output, error) {
				if fail, _ := cmd.Flags().GetBool("fail"); fail {
					return convert.Output{}, errFakeImport
				}
				if input == "missing.input" {
					return convert.Output{}, errFakeImport
				}
				f, err := syntax.BuildFile([]syntax.EntrySpec{{
					Method: "GET",
					URL:    syntax.PlainText("https://example.com/" + input),
				}}, syntax.DialectHurl)
				if err != nil {
					return convert.Output{}, err
				}
				return convert.Output{
					Files:    []convert.GeneratedFile{{Path: input, File: f}},
					Warnings: []convert.Warning{{Kind: "demo", Message: "just a demo warning"}},
				}, nil
			},
		})
	})
}

var errFakeImport = fakeImportError{}

type fakeImportError struct{}

func (fakeImportError) Error() string { return "fake importer: bad input" }

func TestImportUnknownKind(t *testing.T) {
	registerFakeImportKind(t)
	code, _, errOut := runArgs(t, "import", "bogus", "in.json")
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitUsage, errOut)
	}
	if !strings.Contains(errOut, "unknown import kind \"bogus\"") {
		t.Errorf("stderr missing unknown-kind message: %s", errOut)
	}
	if !strings.Contains(errOut, fakeKindName) {
		t.Errorf("stderr should list available kinds including %q: %s", fakeKindName, errOut)
	}
}

func TestImportMissingKind(t *testing.T) {
	registerFakeImportKind(t)
	code, _, errOut := runArgs(t, "import")
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitUsage, errOut)
	}
	if !strings.Contains(errOut, "missing import kind") {
		t.Errorf("stderr = %s", errOut)
	}
}

func TestImportSuccess(t *testing.T) {
	registerFakeImportKind(t)
	dir := t.TempDir()
	code, _, errOut := runArgs(t, "import", fakeKindName, "widgets", "-o", dir)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, "widgets.hurl")); err != nil {
		t.Errorf("expected widgets.hurl to be written: %v", err)
	}
	if !strings.Contains(errOut, "widgets.hurl") {
		t.Errorf("summary should mention the written file: %s", errOut)
	}
	if !strings.Contains(errOut, "just a demo warning") {
		t.Errorf("summary should mention the warning: %s", errOut)
	}
}

func TestImportRunErrorIsUsage(t *testing.T) {
	registerFakeImportKind(t)
	code, _, errOut := runArgs(t, "import", fakeKindName, "missing.input", "-o", t.TempDir())
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitUsage, errOut)
	}
	if !strings.Contains(errOut, "fake importer: bad input") {
		t.Errorf("stderr = %s", errOut)
	}
}

func TestImportFlagErrorIsUsage(t *testing.T) {
	registerFakeImportKind(t)
	code, _, errOut := runArgs(t, "import", fakeKindName, "x", "-o", t.TempDir(), "--fail")
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitUsage, errOut)
	}
	if !strings.Contains(errOut, "fake importer: bad input") {
		t.Errorf("stderr = %s", errOut)
	}
}

func TestImportMissingOutputFlag(t *testing.T) {
	registerFakeImportKind(t)
	code, _, errOut := runArgs(t, "import", fakeKindName, "x")
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitUsage, errOut)
	}
	if !strings.Contains(errOut, "output") {
		t.Errorf("stderr should mention the missing --output flag: %s", errOut)
	}
}

func TestImportConflictWithoutForce(t *testing.T) {
	registerFakeImportKind(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "widgets.hurl"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runArgs(t, "import", fakeKindName, "widgets", "-o", dir)
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitUsage, errOut)
	}
	if !strings.Contains(errOut, "widgets.hurl") || !strings.Contains(errOut, "--force") {
		t.Errorf("stderr should name the conflict and mention --force: %s", errOut)
	}
	got, err := os.ReadFile(filepath.Join(dir, "widgets.hurl"))
	if err != nil || string(got) != "existing" {
		t.Errorf("existing file should be untouched: %q, %v", got, err)
	}

	code, _, errOut = runArgs(t, "import", fakeKindName, "widgets", "-o", dir, "--force")
	if code != ExitOK {
		t.Fatalf("exit code with --force = %d, want %d; stderr=%s", code, ExitOK, errOut)
	}
}

func TestImportDryRun(t *testing.T) {
	registerFakeImportKind(t)
	dir := t.TempDir()
	code, _, errOut := runArgs(t, "import", fakeKindName, "widgets", "-o", dir, "--dry-run")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
	}
	if !strings.Contains(errOut, "would write") {
		t.Errorf("dry run summary should say \"would write\": %s", errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, "widgets.hurl")); err == nil {
		t.Error("dry run should not have written widgets.hurl")
	}
}

func TestReadImportInput(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("curl http://x"))
	if got, err := readImportInput(cmd, "-"); err != nil || string(got) != "curl http://x" {
		t.Errorf("stdin = %q, %v", got, err)
	}
	dir := t.TempDir()
	if _, err := readImportInput(cmd, dir); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("directory err = %v", err)
	}
	if _, err := readImportInput(cmd, filepath.Join(dir, "missing")); err == nil {
		t.Error("missing file: no error")
	}
}
