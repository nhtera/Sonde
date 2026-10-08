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
	"strconv"
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
// python3 interpreter. The venv is keyed by the requirements file's hash
// (venv-<hash12>), so the vendored tree and the next snapshot each get
// their own and never reinstall over each other. requirements-frozen.txt
// has no published hashes to pin against (see docs/conformance.md), so
// "frozen" here means pinned versions, not a verified supply chain; the
// venv is local tooling to run vendored test fixtures, not a production
// dependency.
func setupVenv(hurlRoot string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	reqPath := filepath.Join(hurlRoot, "bin", "requirements-frozen.txt")
	reqHash, err := fileHash(reqPath)
	if err != nil {
		return "", err
	}
	venvDir := filepath.Join(cacheDir, "sonde-conformance", "venv-"+reqHash[:12])
	python := filepath.Join(venvDir, "bin", "python3")
	pip := filepath.Join(venvDir, "bin", "pip")
	marker := filepath.Join(venvDir, ".installed")
	if _, err := os.Stat(marker); err == nil {
		return python, nil
	}

	if _, err := os.Stat(python); err != nil {
		if err := os.MkdirAll(filepath.Dir(venvDir), 0o750); err != nil {
			return "", err
		}
		cmd := exec.Command("python3", "-m", "venv", venvDir) //nolint:gosec // G204: fixed args.
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("creating venv: %w\n%s", err, output)
		}
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

// scriptEnv is the environment every entry point runs with: the shims
// first on PATH, and the binary they exec.
func scriptEnv(shimDir, target string) []string {
	return append(os.Environ(),
		"PATH="+shimDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SONDE_CONFORMANCE_TARGET="+target,
	)
}

// runScript runs one vendored integration script under bash with the shims
// first on PATH, from the root of its tree.
func runScript(shimDir, target, treeRoot, scriptRelPath string) (processResult, time.Duration, error) {
	cmd := exec.Command("bash", scriptRelPath) //nolint:gosec // G204: scriptRelPath comes from vendored fixture discovery, not user input.
	cmd.Dir = treeRoot
	cmd.Env = scriptEnv(shimDir, target)
	return runCaptured(cmd)
}

// runExport runs one export fixture comparison: the hurlfmt shim with
// --out FORMAT on the fixture, from the formatter tree's root, as
// integration/hurlfmt/test_format.py does upstream.
func runExport(shimDir, target, treeRoot, hurlRel, out string) (processResult, time.Duration, error) {
	cmd := exec.Command(filepath.Join(shimDir, "hurlfmt"), "--out", out, hurlRel) //nolint:gosec // G204: vendored fixture path and a fixed format list.
	cmd.Dir = treeRoot
	cmd.Env = scriptEnv(shimDir, target)
	return runCaptured(cmd)
}

// runPTYScript runs a tests_pty script through the upstream two-pty runner
// (integration/term.py: stdout and stderr each on its own raw 24x100 pty)
// via pty-capture.py, then reads back what each pty received.
func runPTYScript(python, driver, integrationDir, shimDir, target, treeRoot, scriptRelPath string) (processResult, time.Duration, error) {
	scratch, err := os.MkdirTemp("", "sonde-conformance-pty-*")
	if err != nil {
		return processResult{}, 0, err
	}
	defer os.RemoveAll(scratch) //nolint:errcheck // best-effort scratch-dir cleanup.
	outPath := filepath.Join(scratch, "stdout")
	errPath := filepath.Join(scratch, "stderr")
	codePath := filepath.Join(scratch, "exit")

	cmd := exec.Command(python, driver, integrationDir, scriptRelPath, outPath, errPath, codePath) //nolint:gosec // G204: harness-owned driver and vendored script path.
	cmd.Dir = treeRoot
	cmd.Env = scriptEnv(shimDir, target)
	driverOut, duration, err := runCaptured(cmd)
	if err != nil {
		return processResult{}, duration, err
	}
	if driverOut.ExitCode == -1 {
		// Timed out: report it like any other script timeout.
		return driverOut, duration, nil
	}
	code, err := os.ReadFile(codePath) //nolint:gosec // G304: our scratch file.
	if err != nil {
		return processResult{}, duration, fmt.Errorf("pty runner failed (exit %d): %s%s", driverOut.ExitCode, driverOut.Stdout, driverOut.Stderr)
	}
	exitCode, err := strconv.Atoi(strings.TrimSpace(string(code)))
	if err != nil {
		return processResult{}, duration, fmt.Errorf("pty runner wrote exit code %q: %w", code, err)
	}
	stdout, _ := os.ReadFile(outPath) //nolint:errcheck,gosec // our scratch file; a read failure surfaces as empty output.
	stderr, _ := os.ReadFile(errPath) //nolint:errcheck,gosec // our scratch file; a read failure surfaces as empty output.
	return processResult{ExitCode: exitCode, Stdout: stdout, Stderr: stderr}, duration, nil
}

// runCaptured runs cmd with no stdin, capturing its stdout/stderr through
// temp files rather than pipes, and kills its whole process group once it
// exceeds scriptTimeout (exit code -1). Piping the reference binary's
// output directly to a Go process can wedge it in uninterruptible sleep
// when it detects a non-terminal stdout; files avoid that entirely and
// match how a real terminal captures output.
func runCaptured(cmd *exec.Cmd) (processResult, time.Duration, error) {
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

	cmd.Stdin = nil // os/exec connects a nil Stdin to the null device.
	cmd.Stdout = outFile
	cmd.Stderr = errFile
	cmd.SysProcAttr = groupAttr()

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
// if set, otherwise a fixed name per target under the OS temp directory
// (…-next-results.json for the pinned snapshot, so the two never overwrite
// each other). It is intentionally outside the vendored trees, which are
// unchanged from upstream.
func resultsPath(next bool) string {
	if p := os.Getenv("SONDE_CONFORMANCE_RESULTS"); p != "" {
		return p
	}
	if next {
		return filepath.Join(os.TempDir(), "sonde-conformance-next-results.json")
	}
	return filepath.Join(os.TempDir(), "sonde-conformance-results.json")
}

// boolEnv reports whether the named environment variable is set to a
// truthy value ("1", "true", case-insensitive).
func boolEnv(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return v == "1" || v == "true"
}
