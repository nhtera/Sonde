// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestExitCode(t *testing.T) {
	base := errors.New("boom")
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, ExitOK},
		{"untyped is usage", base, ExitUsage},
		{"parse", NewExitError(ExitParse, base), ExitParse},
		{"runtime", NewExitError(ExitRuntime, base), ExitRuntime},
		{"assert", NewExitError(ExitAssert, base), ExitAssert},
		{"undefined", NewExitError(ExitUndefined, base), ExitUndefined},
		{"wrapped exit error", fmt.Errorf("outer: %w", NewExitError(ExitRuntime, base)), ExitRuntime},
		{"canceled", context.Canceled, ExitInterrupted},
		{"canceled wins over code", NewExitError(ExitRuntime, context.Canceled), ExitInterrupted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCode(tt.err); got != tt.want {
				t.Errorf("exitCode(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

func TestExitErrorUnwrap(t *testing.T) {
	base := errors.New("boom")
	err := NewExitError(ExitRuntime, base)
	if !errors.Is(err, base) {
		t.Error("errors.Is(ExitError, cause) = false, want true")
	}
	if err.Error() != "boom" {
		t.Errorf("Error() = %q, want %q", err.Error(), "boom")
	}
	if got := NewExitError(ExitRuntime, nil).Error(); got != "exit code 3" {
		t.Errorf("nil cause Error() = %q, want %q", got, "exit code 3")
	}
}

func runArgs(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(context.Background(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersionCommand(t *testing.T) {
	code, out, _ := runArgs(t, "version")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
	for _, prefix := range []string{"sonde ", "commit: ", "built: ", "go: " + runtime.Version(), "\nFeatures: HTTP2"} {
		if !strings.Contains(out, prefix) {
			t.Errorf("version output missing %q:\n%s", prefix, out)
		}
	}
}

func TestVersionRejectsArgs(t *testing.T) {
	code, _, errOut := runArgs(t, "version", "extra")
	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.HasPrefix(errOut, "error: ") {
		t.Errorf("stderr = %q, want error: prefix", errOut)
	}
}

func TestHelpListsCommands(t *testing.T) {
	code, out, _ := runArgs(t, "--help")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
	if !strings.Contains(out, "version") {
		t.Errorf("help output does not list the version command:\n%s", out)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--no-such-flag"},
	} {
		if code, _, _ := runArgs(t, args...); code != ExitUsage {
			t.Errorf("run(%q) exit code = %d, want %d", args, code, ExitUsage)
		}
	}
}

// TestColorNoColorTogetherIsNotAUsageError checks upstream-style silent
// priority: giving both --color and --no-color is not a usage error, and
// --no-color wins regardless of argument order.
func TestColorNoColorTogetherIsNotAUsageError(t *testing.T) {
	for _, args := range [][]string{
		{"--color", "--no-color", "version"},
		{"--no-color", "--color", "version"},
	} {
		code, _, errOut := runArgs(t, args...)
		if code != ExitOK {
			t.Errorf("run(%q) exit code = %d, want %d; stderr=%s", args, code, ExitOK, errOut)
		}
	}
}

// TestUnknownFirstArgIsAFile checks upstream-style dispatch: an arg that is
// not one of check/fmt/version/run is treated as an input FILE, not an
// unknown command, so it fails as a missing file (ExitUsage, "Cannot
// access") rather than an unknown-command error.
func TestUnknownFirstArgIsAFile(t *testing.T) {
	code, _, errOut := runArgs(t, "no-such-command")
	if code != ExitUsage {
		t.Errorf("run([no-such-command]) exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut, "Cannot access 'no-such-command'") {
		t.Errorf("stderr = %q, want it to report the missing file", errOut)
	}
}

func TestInterruptedRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errOut bytes.Buffer
	if code := run(ctx, []string{"version"}, &out, &errOut); code != ExitInterrupted {
		t.Errorf("exit code = %d, want %d", code, ExitInterrupted)
	}
}

// TestInterruptedRunViaStopChannel is the first-Ctrl-C case (code-reviewer
// finding High #1): closing only the stop channel — never ctx itself, the
// way Execute's first SIGINT does — must still exit 130, not whatever the
// run's own outcome was. A stop channel closed before RunAll ever starts
// schedules zero jobs, so the run "succeeds" (no file failed) by every
// measure except this one; before the fix, run() only checked ctx.Err()
// and returned exit 0 here.
func TestInterruptedRunViaStopChannel(t *testing.T) {
	stop := make(chan struct{})
	close(stop)
	ctx := withStop(context.Background(), stop)
	file := writeTemp(t, "ok.hurl", "GET http://127.0.0.1:1/hello\nHTTP 200\n")

	var out, errOut bytes.Buffer
	code := run(ctx, []string{file}, &out, &errOut)
	if code != ExitInterrupted {
		t.Errorf("exit code = %d, want %d (stderr=%s)", code, ExitInterrupted, errOut.String())
	}
}

func TestResolveBuildInfo(t *testing.T) {
	embedded := &debug.BuildInfo{
		Main: debug.Module{Version: "v1.2.3"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc123"},
			{Key: "vcs.time", Value: "2026-01-02T03:04:05Z"},
		},
	}
	tests := []struct {
		name                   string
		ldVersion, ldCommit    string
		ldDate                 string
		info                   *debug.BuildInfo
		wantVer, wantC, wantDt string
	}{
		{"ldflags win", "0.1.0", "deadbeef", "2026-09-25", embedded, "0.1.0", "deadbeef", "2026-09-25"},
		{"embedded fallback", "", "", "", embedded, "v1.2.3", "abc123", "2026-01-02T03:04:05Z"},
		{"devel ignored", "", "", "", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, "dev", "unknown", "unknown"},
		{"no build info", "", "", "", nil, "dev", "unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveBuildInfo(tt.ldVersion, tt.ldCommit, tt.ldDate, tt.info)
			if got.Version != tt.wantVer || got.Commit != tt.wantC || got.Date != tt.wantDt {
				t.Errorf("got %+v, want version=%s commit=%s date=%s", got, tt.wantVer, tt.wantC, tt.wantDt)
			}
			if got.GoVersion != runtime.Version() {
				t.Errorf("GoVersion = %q, want %q", got.GoVersion, runtime.Version())
			}
		})
	}
}

func TestTyped(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		err  error
		want int
	}{
		{nil, ExitOK},
		{boom, ExitUndefined},
		{NewExitError(ExitParse, boom), ExitParse},
		{silentExit(ExitAssert), ExitAssert},
		{context.Canceled, ExitInterrupted},
	}
	for _, c := range cases {
		err := typed(func(*cobra.Command, []string) error { return c.err })(nil, nil)
		if got := exitCode(err); got != c.want {
			t.Errorf("typed(%v): exit %d, want %d", c.err, got, c.want)
		}
	}
	if !isSilent(silentExit(1)) || isSilent(boom) || isSilent(NewExitError(1, boom)) {
		t.Error("isSilent")
	}
}
