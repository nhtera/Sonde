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

// TestCookieJarWrittenAsNamed checks that --cookie-jar writes the path as
// named: through a symbolic link, keeping an existing file's mode.
func TestCookieJarWrittenAsNamed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links and modes")
	}
	dir := t.TempDir()
	file := writeTemp(t, "empty.hurl", "")
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, nil, 0o644); err != nil { //nolint:gosec // G306: the mode under test
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := runArgs(t, file, "--cookie-jar", link); code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the link was replaced")
	}
	data, err := os.ReadFile(target)
	if err != nil || !strings.HasPrefix(string(data), "# Netscape HTTP Cookie File") {
		t.Errorf("target %q, %v", data, err)
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm() != 0o644 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
}
