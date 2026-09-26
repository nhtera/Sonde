// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil { //nolint:gosec // G306: test fixture.
		t.Fatal(err)
	}
}

func TestCheckOracleDefaultExitZero(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "case")

	v := checkOracle(base, processResult{ExitCode: 0})
	if !v.SemanticPass() || !v.FullPass() {
		t.Fatalf("expected pass with no expectation files, got %+v", v)
	}
	if v.ExpectedExit != 0 {
		t.Errorf("ExpectedExit = %d, want 0", v.ExpectedExit)
	}
}

func TestCheckOracleExitMismatch(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "case")
	writeFixture(t, dir, "case.exit", "2\n")

	v := checkOracle(base, processResult{ExitCode: 1})
	if v.SemanticPass() {
		t.Fatalf("expected failure, got %+v", v)
	}
	if v.Reason == "" {
		t.Errorf("expected a mismatch reason")
	}
}

func TestCheckOracleExit255Skips(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "case")
	writeFixture(t, dir, "case.exit", "0\n")

	v := checkOracle(base, processResult{ExitCode: 255})
	if !v.Skip {
		t.Fatalf("expected Skip, got %+v", v)
	}
}

func TestCheckOracleStdoutExact(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "case")
	writeFixture(t, dir, "case.out", "hello\n")

	pass := checkOracle(base, processResult{ExitCode: 0, Stdout: []byte("hello\n")})
	if !pass.SemanticPass() {
		t.Errorf("expected pass, got %+v", pass)
	}

	fail := checkOracle(base, processResult{ExitCode: 0, Stdout: []byte("bye\n")})
	if fail.SemanticPass() {
		t.Errorf("expected failure, got %+v", fail)
	}
}

// The cookie file banner naming sonde matches the upstream one; nothing
// else is rewritten.
func TestCheckOracleCookieBanner(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "case")
	writeFixture(t, dir, "case.out", upstreamCookieBanner+"x\n")
	if r := checkOracle(base, processResult{Stdout: []byte(sondeCookieBanner + "x\n")}); !r.SemanticPass() {
		t.Errorf("sonde banner rejected: %+v", r)
	}
	if r := checkOracle(base, processResult{Stdout: []byte(sondeCookieBanner + "y\n")}); r.SemanticPass() {
		t.Errorf("different content accepted: %+v", r)
	}
}

func TestCheckOracleStdoutPattern(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "case")
	writeFixture(t, dir, "case.out.pattern", "Duration: <<<[0-9.]+>>> seconds\n")

	pass := checkOracle(base, processResult{ExitCode: 0, Stdout: []byte("Duration: 0.5 seconds\n")})
	if !pass.SemanticPass() {
		t.Errorf("expected pass, got %+v", pass)
	}

	fail := checkOracle(base, processResult{ExitCode: 0, Stdout: []byte("Duration: nope seconds\n")})
	if fail.SemanticPass() {
		t.Errorf("expected failure, got %+v", fail)
	}
}

func TestCheckOracleStderrDoesNotAffectSemanticPass(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "case")
	writeFixture(t, dir, "case.err", "boom\n")

	v := checkOracle(base, processResult{ExitCode: 0, Stderr: []byte("different\n")})
	if !v.SemanticPass() {
		t.Errorf("stderr mismatch must not affect semantic pass, got %+v", v)
	}
	if v.FullPass() {
		t.Errorf("expected full-oracle failure, got %+v", v)
	}
}

func TestCheckOracleStderrExactIgnoresCurlDebugAndNewlines(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "case")
	writeFixture(t, dir, "case.err", "** curl debug\r\nboom\r\n")

	v := checkOracle(base, processResult{ExitCode: 0, Stderr: []byte("boom\n")})
	if !v.FullPass() {
		t.Errorf("expected pass, got %+v", v)
	}
}

func TestCheckOracleStderrPattern(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "case")
	writeFixture(t, dir, "case.err.pattern", "error: <<<.*>>>\n")

	v := checkOracle(base, processResult{ExitCode: 0, Stderr: []byte("error: boom\n")})
	if !v.FullPass() {
		t.Errorf("expected pass, got %+v", v)
	}
}
