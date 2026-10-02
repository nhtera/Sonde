// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package example

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/mocksvc"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/sandbox"
)

func TestWrite(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "Sonde")
	dir, err := Write(parent)
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(parent, Name) {
		t.Fatalf("dir = %s", dir)
	}
	for _, f := range []string{"sonde.yaml", "openapi.yaml", "products.hurl", "checkout.hurl"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	entries, _ := os.ReadDir(parent)
	if len(entries) != 1 {
		t.Errorf("parent holds %d entries, want the example only", len(entries))
	}
}

// A second try opens the folder as it is: a change made since stays.
func TestWriteKeepsExisting(t *testing.T) {
	parent := t.TempDir()
	dir, err := Write(parent)
	if err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(dir, "products.hurl")
	if err := os.WriteFile(mine, []byte("GET http://example.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(parent); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(mine); string(b) != "GET http://example.test\n" {
		t.Errorf("products.hurl overwritten: %q", b)
	}
}

// The example's files pass against the mock of its own spec: their
// asserts follow the spec's examples.
func TestExampleRunsAgainstItsMock(t *testing.T) {
	dir, err := Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, err := sandbox.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := mocksvc.New(func(string, any) {}, func() *sandbox.Root { return root }, func(string) {})
	t.Cleanup(m.Stop)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	st, err := m.Start(context.Background(), port)
	if err != nil {
		t.Fatal(err)
	}
	r := engine.NewRunner(engine.Options{})
	defer r.Close()
	for _, f := range []string{"products.hurl", "checkout.hurl"} {
		src, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		// sonde.yaml's variables, base_url the mock's (as while it runs).
		text := strings.NewReplacer("{{base_url}}", st.URL, "{{sku}}", "TEA-EARL-250").Replace(string(src))
		res, err := r.RunSource(context.Background(), f, []byte(text))
		if err != nil || !res.Success {
			t.Errorf("%s: %+v %v", f, res, err)
		}
	}
}
