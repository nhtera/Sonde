// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nhtera/sonde/internal/sandbox"
)

// TestOptionFilesDotDotAndSymlinkDenied is
// TestOptionFilesOutsideRootDenied's counterpart for the two escape
// shapes that test does not exercise (it only tries an absolute path
// outside the root): a `..`-relative name, and a symbolic link inside the
// root pointing outside it. Both are request-file-given paths (never a
// command line one), so both must go through the sandbox and be denied.
func TestOptionFilesDotDotAndSymlinkDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "file.pem")
	if err := os.WriteFile(outside, certPEM(t, srv), 0o600); err != nil {
		t.Fatal(err)
	}

	rootDir := t.TempDir()
	box, err := sandbox.Open(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	if err := os.Symlink(outside, filepath.Join(rootDir, "link.pem")); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(rootDir, outside)
	if err != nil {
		t.Fatal(err)
	}

	c, err := NewClient(ClientConfig{Sandbox: box, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	for name, path := range map[string]string{"dotdot": rel, "symlink": "link.pem"} {
		t.Run(name, func(t *testing.T) {
			_, err := c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: srv.URL}, &Options{CACert: path})
			var herr *Error
			if !asError(err, &herr) || herr.Kind != ErrFileAccess {
				t.Errorf("CACert=%q: err = %v, want kind ErrFileAccess", path, err)
			}
		})
	}
}

// TestUnixSocketEscapeDenied checks a request-file `unix-socket` option
// cannot name a path outside the file root, through `..` or a symlink,
// the same way cert/key/netrc-file cannot (internal/httpx/dial.go).
func TestUnixSocketEscapeDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	outsideDir := t.TempDir()
	outsideSock := filepath.Join(outsideDir, "evil.sock")

	rootDir := t.TempDir()
	box, err := sandbox.Open(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	if err := os.Symlink(outsideSock, filepath.Join(rootDir, "link.sock")); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(rootDir, outsideSock)
	if err != nil {
		t.Fatal(err)
	}

	c, err := NewClient(ClientConfig{Sandbox: box, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	for name, sock := range map[string]string{"dotdot": rel, "symlink": "link.sock"} {
		t.Run(name, func(t *testing.T) {
			_, err := c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: "http://unix.invalid/"}, &Options{UnixSocket: sock})
			if !errors.Is(err, sandbox.ErrDenied) {
				t.Errorf("UnixSocket=%q: err = %v, want a sandbox.ErrDenied", sock, err)
			}
		})
	}
}
