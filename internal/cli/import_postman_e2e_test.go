// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hurlFiles returns every ".hurl" file under dir, recursively.
func hurlFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".hurl") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

// postmanFixture returns the absolute path of one hand-authored fixture
// under testdata/convert/postman.
func postmanFixture(t *testing.T, rel string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("../../testdata/convert/postman", rel))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// TestE2EImportPostmanFixturesCheck imports every hand-authored fixture
// (both --group values) and checks every generated file parses, proving
// Import always produces valid Sonde source (docs/guides/import-export.md's
// "every imported fixture passes sonde check").
func TestE2EImportPostmanFixturesCheck(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		group   string
	}{
		{"basic.json", "request"},
		{"basic.json", "folder"},
		{"graphql.json", "request"},
		{"auth-types.json", "request"},
		{"folder-group.json", "folder"},
		{"v2-compat.json", "request"},
	} {
		dir := t.TempDir()
		code, _, errOut := runArgs(t, "import", "postman", postmanFixture(t, tc.fixture), "-o", dir, "--group", tc.group)
		if code != ExitOK {
			t.Fatalf("%s --group %s: import exit %d\n%s", tc.fixture, tc.group, code, errOut)
		}
		files := hurlFiles(t, dir)
		if len(files) == 0 {
			t.Fatalf("%s --group %s: no .hurl files written", tc.fixture, tc.group)
		}
		if code, _, errOut := runArgs(t, append([]string{"check"}, files...)...); code != ExitOK {
			t.Errorf("%s --group %s: check exit %d\n%s", tc.fixture, tc.group, code, errOut)
		}
	}
}

// TestE2EImportPostmanRunsGreen imports basic.json, points its sonde.yaml
// "collection" environment's base_url at a local server (every other value
// it needs comes from the environment or --variable/--secret) and runs
// every generated request, proving the sonde.yaml Import wrote actually
// loads and the requests it describes are real, working requests.
func TestE2EImportPostmanRunsGreen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok": true}`)
	}))
	t.Cleanup(srv.Close)

	dir := filepath.Join(t.TempDir(), "out")
	code, _, errOut := runArgs(t, "import", "postman", postmanFixture(t, "basic.json"), "-o", dir)
	if code != ExitOK {
		t.Fatalf("import: exit %d\n%s", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, "sonde.yaml")); err != nil {
		t.Fatalf("sonde.yaml was not written: %v", err)
	}
	// Upload.hurl's multipart file field reads this file (a name reference
	// only: Import never touches it).
	if err := os.WriteFile(filepath.Join(dir, "photo.png"), []byte("fake-png"), 0o600); err != nil {
		t.Fatal(err)
	}

	files := hurlFiles(t, dir)
	if len(files) == 0 {
		t.Fatal("no .hurl files written")
	}

	args := append([]string{
		"--test", "--env", "collection",
		"--variable", "base_url=" + srv.URL,
		"--variable", "token=t",
		"--variable", "widget_name=gadget",
		"--variable", "title=photo",
		"--variable", "b=2",
		"--secret", "admin_password=p4ss",
	}, files...)
	if code, _, errOut := runArgs(t, args...); code != ExitOK {
		t.Fatalf("run: exit %d\n%s", code, errOut)
	}

	// sonde.yaml was really discovered and parsed: an --env naming an
	// environment that does not exist is a clear, named error.
	code, _, errOut = runArgs(t, "--env", "no-such-env", files[0])
	if code != ExitUsage || !strings.Contains(errOut, "no-such-env") {
		t.Errorf("bad --env: exit %d\n%s", code, errOut)
	}
}
