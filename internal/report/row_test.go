// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nhtera/sonde/engine"
)

// runRows runs a file echoing a row secret once per row, failing its
// assert so that the secret lands in error messages too.
func runRows(t *testing.T) (*engine.Runner, []*engine.UnitResult) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "token="+r.Header.Get("X-Token")) //nolint:gosec // G705: test server echoing its input
	}))
	t.Cleanup(srv.Close)
	src := []byte("GET " + srv.URL + "\nX-Token: {{token}}\nHTTP 200\n[Asserts]\nbody == \"nope\"\n")
	var jobs []engine.Job
	for i, token := range []string{"row-secret-one", "row-secret-two"} {
		jobs = append(jobs, engine.Job{Name: "api.hurl", Source: src, Row: &engine.Row{
			Index: i + 1, Secrets: map[string]string{"token": token},
		}})
	}
	r := engine.NewRunner(engine.Options{})
	results := make([]*engine.UnitResult, len(jobs))
	r.RunAll(context.Background(), slices.Values(jobs), engine.RunAllOptions{
		Finished: func(seq int, _ engine.Job, res *engine.UnitResult, err error) bool {
			if err != nil {
				t.Fatal(err)
			}
			results[seq] = res
			return true
		},
	})
	return r, results
}

func TestReportsRows(t *testing.T) {
	r, results := runRows(t)
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(WriteJUnit(filepath.Join(dir, "junit.xml"), results, r.Redact))
	must(WriteTAP(filepath.Join(dir, "report.tap"), results, r.Redact))
	must(WriteJSON(filepath.Join(dir, "json"), results, r.Redact))
	must(WriteHTML(filepath.Join(dir, "html"), results, r.Redact))

	var all strings.Builder
	must(filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path) //nolint:gosec // G304: test temp dir
		all.Write(b)
		return err
	}))
	for _, secret := range []string{"row-secret-one", "row-secret-two"} {
		if strings.Contains(all.String(), secret) {
			t.Errorf("a report leaks %q", secret)
		}
	}
	for _, want := range []string{
		`name="api.hurl#row-2"`,           // JUnit
		`not ok 1 - api.hurl\#row-1`,      // TAP escapes "#"
		`"sonde":{"iteration":{"row":2}}`, // JSON
		"api.hurl#row-1",                  // HTML
	} {
		if !strings.Contains(all.String(), want) {
			t.Errorf("reports lack %q", want)
		}
	}
}
