// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvFilePath(t *testing.T) {
	if got, ok := (Env{"XDG_CONFIG_HOME": "/xdg"}).FilePath(); !ok || got != filepath.Join("/xdg", "hurl", "config") {
		t.Errorf("FilePath(XDG) = %q, %v", got, ok)
	}
	if got, ok := (Env{"HOME": "/home/u"}).FilePath(); !ok || got != filepath.Join("/home/u", "config", "hurl", "config") {
		t.Errorf("FilePath(HOME) = %q, %v", got, ok)
	}
	// XDG_CONFIG_HOME wins when both are set.
	if got, ok := (Env{"XDG_CONFIG_HOME": "/xdg", "HOME": "/home/u"}).FilePath(); !ok || got != filepath.Join("/xdg", "hurl", "config") {
		t.Errorf("FilePath(both) = %q, %v", got, ok)
	}
	if _, ok := (Env{}).FilePath(); ok {
		t.Error("FilePath(none) should report not set")
	}
}

func TestParseFile(t *testing.T) {
	content := "# ignore\n\n--verbose\n--header=header1:value1\n--header user-agent2:value2\n" +
		"--max-redirs 10\n--user-agent=\"Mozilla/5.0 A\"\n"
	opts, err := ParseFile(content)
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Verbose {
		t.Error("Verbose = false, want true")
	}
	if len(opts.Headers) != 2 || opts.Headers[0] != "header1:value1" || opts.Headers[1] != "user-agent2:value2" {
		t.Errorf("Headers = %v", opts.Headers)
	}
	if opts.MaxRedirs == nil || *opts.MaxRedirs != 10 {
		t.Errorf("MaxRedirs = %v, want 10", opts.MaxRedirs)
	}
	if opts.UserAgent == nil || *opts.UserAgent != "Mozilla/5.0 A" {
		t.Errorf("UserAgent = %v, want Mozilla/5.0 A", opts.UserAgent)
	}
}

func TestParseFileHeaderWithDashesValue(t *testing.T) {
	opts, err := ParseFile("--header --test:1\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(opts.Headers) != 1 || opts.Headers[0] != "--test:1" {
		t.Errorf("Headers = %v, want [--test:1]", opts.Headers)
	}
}

func TestParseFileErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"not an option", "verbose\n"},
		{"unknown option", "--xxx\n"},
		{"verbose with value", "--verbose=1\n"},
		{"max-redirs not an int", "--max-redirs=a\n"},
		{"max-redirs too small", "--max-redirs=-2\n"},
		{"header missing value", "--header\n"},
		{"header empty value", "--header=\n"},
		{"user-agent missing value", "--user-agent\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseFile(tt.content); err == nil {
				t.Fatalf("ParseFile(%q): expected an error", tt.content)
			}
		})
	}
}

func isZeroFileOptions(o FileOptions) bool {
	return !o.Verbose && o.Headers == nil && o.MaxRedirs == nil && o.UserAgent == nil
}

func TestLoadConfigFile(t *testing.T) {
	if opts, err := LoadConfigFile(""); err != nil || !isZeroFileOptions(opts) {
		t.Errorf("LoadConfigFile(\"\") = %+v, %v", opts, err)
	}
	if opts, err := LoadConfigFile(filepath.Join(t.TempDir(), "absent")); err != nil || !isZeroFileOptions(opts) {
		t.Errorf("LoadConfigFile(missing) = %+v, %v", opts, err)
	}

	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("--verbose\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Verbose {
		t.Error("Verbose = false, want true")
	}
}

func TestLoadConfigFileParseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("--xxx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfigFile(path); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestLoadConfigFileUnreadable(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "config")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfigFile(sub); err == nil {
		t.Fatal("expected an error reading a directory as a config file")
	}
}
