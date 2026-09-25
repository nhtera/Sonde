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
	for _, prefix := range []string{"sonde ", "commit: ", "built: ", "go: " + runtime.Version()} {
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
		{"no-such-command"},
		{"--no-such-flag"},
		{"--color", "--no-color", "version"},
	} {
		if code, _, _ := runArgs(t, args...); code != ExitUsage {
			t.Errorf("run(%q) exit code = %d, want %d", args, code, ExitUsage)
		}
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
