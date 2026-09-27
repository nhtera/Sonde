// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestE2EMultipartFileEscape is TestE2EFileRootEscape's counterpart for a
// `[Multipart]` file part (engine/request.go's readBodyFile): a request
// file cannot reach a file outside the file root through that part
// either, whether by `..` or by a symbolic link.
func TestE2EMultipartFileEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	srv := testServer(t)
	outside := writeTemp(t, "outside.txt", "outside")
	root := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(root, outside)
	if err != nil {
		t.Fatal(err)
	}
	for name, part := range map[string]string{"dotdot": rel, "symlink": "link.txt"} {
		t.Run(name, func(t *testing.T) {
			file := writeTemp(t, "escape.hurl",
				"POST "+srv.URL+"/hello\n[Multipart]\nf: file,"+part+";\nHTTP 200\n")
			code, _, errOut := runArgs(t, file, "--file-root", root)
			if code != ExitRuntime || !strings.Contains(errOut, "unauthorized access to file") {
				t.Errorf("exit code = %d, want %d and a denied file access; stderr=%s", code, ExitRuntime, errOut)
			}
		})
	}
}

// TestE2EOutputEscapeVariants is TestE2EExitCodes's "runtime" case
// (an `output: ../../out.txt` request-file option) extended to the two
// escape shapes it does not cover: an absolute path outside the file
// root, and a symbolic link inside the root pointing outside it. Both
// must be denied, and neither may leave a file behind at the escaped
// target.
func TestE2EOutputEscapeVariants(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	srv := testServer(t)
	root := t.TempDir()
	outsideDir := t.TempDir()
	absTarget := filepath.Join(outsideDir, "abs-out.txt")
	linkTarget := filepath.Join(outsideDir, "link-out.txt")
	if err := os.Symlink(linkTarget, filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		option, watch string
	}{
		"absolute": {option: absTarget, watch: absTarget},
		"symlink":  {option: "link.txt", watch: linkTarget},
	} {
		t.Run(name, func(t *testing.T) {
			file := writeTemp(t, "out.hurl",
				"GET "+srv.URL+"/hello\n[Options]\noutput: "+tc.option+"\nHTTP 200\n")
			code, _, errOut := runArgs(t, file, "--file-root", root)
			if code != ExitRuntime {
				t.Errorf("exit code = %d, want %d; stderr=%s", code, ExitRuntime, errOut)
			}
			if _, err := os.Stat(tc.watch); !os.IsNotExist(err) {
				t.Errorf("output escaped the file root: %s now exists", tc.watch)
			}
		})
	}
}
