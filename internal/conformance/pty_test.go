// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRunPTYScriptSeparatesStreams runs a script through pty-capture.py and
// the vendored upstream term.py: stdout and stderr must arrive on separate
// terminals (never merged), both must be ttys, and the size must be 24x100.
func TestRunPTYScriptSeparatesStreams(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX pty")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not on PATH")
	}
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	tree := t.TempDir()
	writeScript(t, tree, "tests_pty/x/x.sh", `exec 3>&1
[ -t 1 ] && echo "out tty $(stty size <&3)"
[ -t 2 ] && echo "err tty $(stty size <&2)" >&2
exit 3
`)

	got, _, err := runPTYScript(python, filepath.Join(root, "internal", "conformance", "pty-capture.py"),
		filepath.Join(root, "testdata", "conformance", "integration"), t.TempDir(), "unused", tree, "tests_pty/x/x.sh")
	if err != nil {
		t.Fatal(err)
	}
	if got.ExitCode != 3 {
		t.Errorf("exit = %d, want 3", got.ExitCode)
	}
	// Raw mode: no "\r\n" translation on either stream.
	if string(got.Stdout) != "out tty 24 100\n" {
		t.Errorf("stdout = %q, want %q", got.Stdout, "out tty 24 100\n")
	}
	if string(got.Stderr) != "err tty 24 100\n" || strings.Contains(string(got.Stdout), "err") {
		t.Errorf("stderr = %q, stdout = %q: want the streams on separate ptys", got.Stderr, got.Stdout)
	}
}
