// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// repoRoot locates the module root from this source file's own path, so
// the harness works regardless of the directory `go test` happens to run
// in.
func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("conformance: could not determine source file location")
	}
	// This file lives at <root>/internal/conformance/runner.go.
	return filepath.Abs(filepath.Join(filepath.Dir(file), "..", ".."))
}

// buildSondeBinary compiles ./cmd/sonde into destDir/sonde and returns its
// path. Building fresh (rather than reusing bin/sonde) keeps the
// conformance run independent of whatever `make build` last produced.
func buildSondeBinary(destDir string) (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", err
	}
	out := filepath.Join(destDir, "sonde")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/sonde") //nolint:gosec // G204: fixed args.
	cmd.Dir = root
	cmd.Env = os.Environ()
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("building ./cmd/sonde: %w\n%s", err, output)
	}
	return out, nil
}

// setupVenv ensures a Python virtualenv with the reference servers'
// dependencies exists under the user's cache directory, and returns its
// python3 interpreter. requirements-frozen.txt has no published hashes to
// pin against (see docs/conformance.md), so "frozen" here means pinned
// versions, not a verified supply chain; the venv is local tooling to run
// vendored test fixtures, not a production dependency.
func setupVenv(hurlRoot string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	venvDir := filepath.Join(cacheDir, "sonde-conformance", "venv")
	python := filepath.Join(venvDir, "bin", "python3")
	pip := filepath.Join(venvDir, "bin", "pip")

	if _, err := os.Stat(python); err != nil {
		if err := os.MkdirAll(filepath.Dir(venvDir), 0o750); err != nil {
			return "", err
		}
		cmd := exec.Command("python3", "-m", "venv", venvDir) //nolint:gosec // G204: fixed args.
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("creating venv: %w\n%s", err, output)
		}
	}

	reqPath := filepath.Join(hurlRoot, "bin", "requirements-frozen.txt")
	reqHash, err := fileHash(reqPath)
	if err != nil {
		return "", err
	}
	marker := filepath.Join(venvDir, ".requirements-hash")
	if installed, _ := os.ReadFile(marker); string(installed) == reqHash { //nolint:errcheck // absent marker just means "install".
		return python, nil
	}

	cmd := exec.Command(pip, "install", "--quiet", "--requirement", reqPath) //nolint:gosec // G204: fixed args.
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pip install: %w\n%s", err, output)
	}
	if err := os.WriteFile(marker, []byte(reqHash), 0o644); err != nil { //nolint:gosec // G306: non-sensitive cache marker.
		return "", err
	}
	return python, nil
}

func fileHash(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // G304: fixed, vendored path.
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// scriptTimeout bounds a single script's wall-clock time. Scripts that hit
// it are recorded as a failure (harness-level, not oracle-level) rather
// than hanging the suite.
const scriptTimeout = 60 * time.Second

// runScript runs one vendored integration script under bash with the shim
// first on PATH, capturing its stdout/stderr through temp files rather
// than pipes. Piping the reference binary's output directly to a Go
// process can wedge it in uninterruptible sleep when it detects a
// non-terminal stdout; files avoid that entirely and match how a real
// terminal captures output.
func runScript(shimDir, target, hurlRoot, scriptRelPath string) (processResult, time.Duration, error) {
	outFile, err := os.CreateTemp("", "sonde-conformance-out-*")
	if err != nil {
		return processResult{}, 0, err
	}
	defer os.Remove(outFile.Name()) //nolint:errcheck // best-effort scratch-file cleanup.
	defer outFile.Close()           //nolint:errcheck // closed after being read back below.

	errFile, err := os.CreateTemp("", "sonde-conformance-err-*")
	if err != nil {
		return processResult{}, 0, err
	}
	defer os.Remove(errFile.Name()) //nolint:errcheck // best-effort scratch-file cleanup.
	defer errFile.Close()           //nolint:errcheck // closed after being read back below.

	cmd := exec.Command("bash", scriptRelPath) //nolint:gosec // G204: scriptRelPath comes from vendored fixture discovery, not user input.
	cmd.Dir = hurlRoot
	cmd.Stdin = nil // os/exec connects a nil Stdin to the null device.
	cmd.Stdout = outFile
	cmd.Stderr = errFile
	cmd.Env = append(os.Environ(),
		"PATH="+shimDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SONDE_CONFORMANCE_TARGET="+target,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return processResult{}, 0, err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	timedOut := false
	select {
	case <-done:
	case <-time.After(scriptTimeout):
		timedOut = true
		killGroup(cmd.Process.Pid, syscall.SIGKILL)
		<-done
	}
	duration := time.Since(start)

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	if timedOut {
		exitCode = -1
	}

	stdout, _ := os.ReadFile(outFile.Name()) //nolint:errcheck // scratch file we just wrote; a read failure surfaces as empty output.
	stderr, _ := os.ReadFile(errFile.Name()) //nolint:errcheck // scratch file we just wrote; a read failure surfaces as empty output.
	if timedOut {
		stderr = append(stderr, []byte(fmt.Sprintf("\n[conformance harness] killed after exceeding %s\n", scriptTimeout))...)
	}

	return processResult{ExitCode: exitCode, Stdout: stdout, Stderr: stderr}, duration, nil
}

// resultsPath resolves where results.json is written: SONDE_CONFORMANCE_RESULTS
// if set, otherwise a fixed name under the OS temp directory. It is
// intentionally outside testdata/conformance/hurl, which is vendored and
// unchanged from upstream.
func resultsPath() string {
	if p := os.Getenv("SONDE_CONFORMANCE_RESULTS"); p != "" {
		return p
	}
	return filepath.Join(os.TempDir(), "sonde-conformance-results.json")
}

// boolEnv reports whether the named environment variable is set to a
// truthy value ("1", "true", case-insensitive).
func boolEnv(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return v == "1" || v == "true"
}
