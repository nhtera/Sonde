// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runplan

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/datarow"
)

// sent runs src's single entry with opts and job and returns the X-Token
// header it sent and whether the run's redaction masks value.
func sent(t *testing.T, opts engine.Options, job engine.Job, value string) (string, bool) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(srv.Close)
	if opts.Variables == nil {
		opts.Variables = map[string]any{}
	}
	opts.Variables["base"] = srv.URL
	job.Name = filepath.Join(t.TempDir(), "t.hurl")
	job.Source = []byte("GET {{base}}\nX-Token: {{token}}\nHTTP 200\n")
	r := engine.NewRunner(opts)
	var res *engine.UnitResult
	r.RunAll(context.Background(), slices.Values([]engine.Job{job}), engine.RunAllOptions{
		Finished: func(_ int, _ engine.Job, u *engine.UnitResult, err error) bool {
			if err != nil {
				t.Error(err)
			}
			res = u
			return true
		},
	})
	if res == nil || !res.Success {
		t.Fatalf("run failed: %+v", res)
	}
	for _, h := range res.Entries[0].Calls[0].Request.Headers {
		if h.Name == "X-Token" {
			return h.Value, res.Redact(value) != value
		}
	}
	return "", false
}

// TestCaptureCollisions pins the layer model of a rerun with captures.
func TestCaptureCollisions(t *testing.T) {
	t.Run("plain capture over a secrets-file stub", func(t *testing.T) {
		opts := engine.Options{Secrets: map[string]string{"token": "stub"}}
		job := engine.Job{}
		ApplyCaptures(&opts, &job, map[string]any{"token": "live"}, nil)
		if v, masked := sent(t, opts, job, "live"); v != "live" || masked {
			t.Errorf("sent %q, masked %v", v, masked)
		}
	})
	t.Run("redacted capture over a secrets-file stub", func(t *testing.T) {
		opts := engine.Options{Secrets: map[string]string{"token": "stub"}}
		job := engine.Job{}
		ApplyCaptures(&opts, &job, nil, map[string]string{"token": "live"})
		if v, masked := sent(t, opts, job, "live"); v != "live" || !masked {
			t.Errorf("sent %q, masked %v", v, masked)
		}
	})
	t.Run("plain capture over a project secret", func(t *testing.T) {
		opts := engine.Options{}
		job := engine.Job{Secrets: map[string]string{"token": "project"}}
		projectSecrets := job.Secrets
		ApplyCaptures(&opts, &job, map[string]any{"token": "live"}, nil)
		if v, masked := sent(t, opts, job, "live"); v != "live" || masked {
			t.Errorf("sent %q, masked %v", v, masked)
		}
		if projectSecrets["token"] != "project" {
			t.Error("ApplyCaptures changed the caller's map")
		}
	})
	t.Run("redacted capture over an override", func(t *testing.T) {
		opts := engine.Options{Variables: map[string]any{"token": "override"}}
		job := engine.Job{}
		ApplyCaptures(&opts, &job, nil, map[string]string{"token": "live"})
		if v, masked := sent(t, opts, job, "live"); v != "live" || !masked {
			t.Errorf("sent %q, masked %v", v, masked)
		}
	})
	t.Run("data-secret column and a redacted capture", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "rows.csv")
		if err := os.WriteFile(path, []byte("token,user\nrow-token,bob\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		secret := map[string]string{"token": "live"}
		// The capture's name goes in the overridden names only: as a
		// secret it would clash with the column.
		if _, err := datarow.Open(path, []string{"token"}, nil, secret); err == nil {
			t.Fatal("a column named like a secret is not refused: the test proves nothing")
		}
		d, err := datarow.Open(path, []string{"token"}, []string{"token"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		opts, job := engine.Options{}, engine.Job{}
		ApplyCaptures(&opts, &job, nil, secret)
		_, _ = d.Each(func(row *engine.Row) bool {
			if _, ok := row.Secrets["token"]; ok {
				t.Error("the row's column wins over the capture")
			}
			job.Row = row
			return false
		})
		if v, masked := sent(t, opts, job, "live"); v != "live" || !masked {
			t.Errorf("sent %q, masked %v", v, masked)
		}
	})
}
