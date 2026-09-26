// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package openapi

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestValidate31NoFileAccess checks that $schema and $dynamicRef targets
// are never read: a FIFO target would block forever.
func TestValidate31NoFileAccess(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip(err)
	}
	spec := fmt.Sprintf(`openapi: 3.1.0
info: {title: t, version: "1"}
paths:
  /a:
    get:
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema: {type: object, $schema: "file://%[1]s", $dynamicRef: "file://%[1]s"}
`, fifo)
	path := filepath.Join(dir, "spec.yaml")
	if err := os.WriteFile(path, []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(context.Background(), path, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan []string, 1)
	go func() { done <- check(s.Validator(Options{}), "GET", "http://x/a", 200, `{}`) }()
	select {
	case got := <-done:
		expect(t, got)
	case <-time.After(5 * time.Second):
		t.Fatal("validation blocked on a local file")
	}
	// A FIFO as a $ref target is refused instead of opened.
	if err := os.WriteFile(path, []byte("openapi: 3.0.3\ninfo: {title: t, version: '1'}\npaths:\n  /a: {$ref: fifo}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	go func() {
		_, err := Load(context.Background(), path, LoadOptions{})
		done <- []string{fmt.Sprint(err)}
	}()
	select {
	case got := <-done:
		if !strings.Contains(got[0], "not a regular file") {
			t.Errorf("FIFO $ref: %v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Load blocked on a FIFO $ref")
	}
}
