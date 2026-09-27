// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/nhtera/sonde/internal/value"
)

const testdataConfig = "../../testdata/config"

func TestLoadProjectHappyPath(t *testing.T) {
	p, err := LoadProject(filepath.Join(testdataConfig, "happy/sonde.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 1 {
		t.Errorf("Version = %d, want 1", p.Version)
	}
	if p.Defaults.Env != "local" || p.Defaults.Jobs != 4 {
		t.Errorf("Defaults = %+v, want {local 4}", p.Defaults)
	}
	local, ok := p.Environments["local"]
	if !ok {
		t.Fatal(`environments["local"] missing`)
	}
	if local.Variables["retries"] != value.Int(3) {
		t.Errorf("retries = %#v, want 3", local.Variables["retries"])
	}
	if local.Variables["ratio"] != value.Float(0.5) {
		t.Errorf("ratio = %#v, want 0.5", local.Variables["ratio"])
	}
	if local.Variables["strict"] != value.Bool(true) {
		t.Errorf("strict = %#v, want true", local.Variables["strict"])
	}
	if _, isNull := local.Variables["extra"].(value.Null); !isNull {
		t.Errorf("extra = %#v, want null", local.Variables["extra"])
	}
	if len(local.VariablesFiles) != 1 || local.VariablesFiles[0] != "env/local.vars" {
		t.Errorf("VariablesFiles = %v", local.VariablesFiles)
	}
	if len(local.SecretsFiles) != 1 || local.SecretsFiles[0] != "env/local.secrets" {
		t.Errorf("SecretsFiles = %v", local.SecretsFiles)
	}
	if _, ok := p.Environments["staging"]; !ok {
		t.Error(`environments["staging"] missing`)
	}
}

func TestLoadProjectUnknownKey(t *testing.T) {
	_, err := LoadProject(filepath.Join(testdataConfig, "errors/unknown-key.yaml"))
	if err == nil {
		t.Fatal("expected an error for an unknown top-level key")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error = %q, want it to name line 2", err)
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error = %q, want it to name the unknown key", err)
	}
}

func TestLoadProjectBadVersion(t *testing.T) {
	_, err := LoadProject(filepath.Join(testdataConfig, "errors/bad-version.yaml"))
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("err = %v, want a version error", err)
	}
}

func TestLoadProjectMissingVersion(t *testing.T) {
	_, err := LoadProject(filepath.Join(testdataConfig, "errors/missing-version.yaml"))
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("err = %v, want a version error", err)
	}
}

func TestLoadProjectBadVariableType(t *testing.T) {
	_, err := LoadProject(filepath.Join(testdataConfig, "errors/bad-type.yaml"))
	if err == nil {
		t.Fatal("expected an error for a list-valued variable")
	}
	if !strings.Contains(err.Error(), "line") {
		t.Errorf("error = %q, want it to name a line", err)
	}
}

func TestLoadProjectNegativeJobs(t *testing.T) {
	_, err := LoadProject(filepath.Join(testdataConfig, "errors/negative-jobs.yaml"))
	if err == nil {
		t.Fatal("expected an error for a negative defaults.jobs")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("error = %q, want it to name line 3", err)
	}
	if !strings.Contains(err.Error(), "jobs") {
		t.Errorf("error = %q, want it to name defaults.jobs", err)
	}
}

func TestLoadProjectJobsTooLarge(t *testing.T) {
	_, err := LoadProject(filepath.Join(testdataConfig, "errors/jobs-too-large.yaml"))
	if err == nil {
		t.Fatal("expected an error for defaults.jobs above the cap")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("error = %q, want it to name line 3", err)
	}
}

func TestLoadProjectMultiDocument(t *testing.T) {
	_, err := LoadProject(filepath.Join(testdataConfig, "errors/multi-document.yaml"))
	if err == nil {
		t.Fatal("expected an error for a second YAML document")
	}
	if !strings.Contains(err.Error(), "one YAML document") {
		t.Errorf("error = %q, want it to mention the single-document rule", err)
	}
}

func TestLoadProjectNotRegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ProjectFileName)
	if err := os.Mkdir(path, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProject(path); err == nil {
		t.Fatal("expected an error for a sonde.yaml that is a directory")
	}
}

func TestLoadProjectTooLarge(t *testing.T) {
	dir := t.TempDir()
	big := "version: 1\n# " + strings.Repeat("x", maxProjectFileBytes+1) + "\n"
	writeProjectFile(t, dir, big)
	if _, err := LoadProject(filepath.Join(dir, ProjectFileName)); err == nil {
		t.Fatal("expected an error for a sonde.yaml over the size cap")
	}
}

func TestLoadProjectReferencedFileTooLarge(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "big.env"), []byte(strings.Repeat("a=1\n", 1)+strings.Repeat("x", maxProjectFileBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, dir, "version: 1\nenvironments:\n  local:\n    variables_files: [\"big.env\"]\n")
	p, err := LoadProject(filepath.Join(dir, ProjectFileName))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.Resolve("local"); err == nil {
		t.Fatal("expected an error for a variables_files entry over the size cap")
	}
}

func TestLoadProjectReferencedFileNotRegular(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FIFOs are not portable to Windows")
	}
	dir := t.TempDir()
	fifo := filepath.Join(dir, "secrets.fifo")
	if err := syscallMkfifo(fifo); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}
	writeProjectFile(t, dir, "version: 1\nenvironments:\n  local:\n    secrets_files: [\"secrets.fifo\"]\n")
	p, err := LoadProject(filepath.Join(dir, ProjectFileName))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.Resolve("local"); err == nil {
		t.Fatal("expected an error for a secrets_files entry that is not a regular file")
	}
}

func TestLoadProjectMissingFile(t *testing.T) {
	if _, err := LoadProject(filepath.Join(t.TempDir(), "sonde.yaml")); err == nil {
		t.Fatal("expected an error for a missing sonde.yaml")
	}
}

func TestLoadProjectPathEscapeDotDot(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "version: 1\nenvironments:\n  local:\n    variables_files: [\"../secret.env\"]\n")
	if _, err := LoadProject(filepath.Join(dir, ProjectFileName)); err == nil {
		t.Fatal("expected an error for a variables_files path escaping via ..")
	}
}

func TestLoadProjectPathEscapeAbsolute(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "version: 1\nenvironments:\n  local:\n    secrets_files: [\"/etc/passwd\"]\n")
	if _, err := LoadProject(filepath.Join(dir, ProjectFileName)); err == nil {
		t.Fatal("expected an error for an absolute secrets_files path")
	}
}

func TestLoadProjectPathEscapeSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(dir, "escaped")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	writeProjectFile(t, dir, "version: 1\nenvironments:\n  local:\n    variables_files: [\"escaped/vars.env\"]\n")
	if _, err := LoadProject(filepath.Join(dir, ProjectFileName)); err == nil {
		t.Fatal("expected an error for a variables_files path escaping through a symlink")
	}
}

func TestLoadProjectOpenAPI(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "version: 1\nopenapi:\n  spec: api/openapi.yaml\n  server: http://localhost:3000/v1\n  strict: true\n"+
		"  exclude_operations: [\"GET /health\"]\n  exclude_files: [\"legacy/**/*.hurl\"]\n")
	p, err := LoadProject(filepath.Join(dir, ProjectFileName))
	if err != nil {
		t.Fatal(err)
	}
	o := p.OpenAPI
	if o == nil || !filepath.IsAbs(o.Spec) || !strings.HasSuffix(o.Spec, filepath.Join("api", "openapi.yaml")) ||
		o.Server != "http://localhost:3000/v1" || !o.Strict || len(o.ExcludeOperations) != 1 {
		t.Fatalf("OpenAPI = %+v", o)
	}
	for file, want := range map[string]bool{
		"legacy/a.hurl":        true,
		"legacy/x/y/b.hurl":    true,
		"legacy/a.sonde":       false,
		"api/legacy/a.hurl":    false,
		"../outside/a.hurl":    false,
		"legacy/sub/deep.hurl": true,
	} {
		if got := o.ExcludesFile(dir, filepath.Join(dir, file)); got != want {
			t.Errorf("ExcludesFile(%s) = %v", file, got)
		}
	}
}

func TestLoadProjectOpenAPIErrors(t *testing.T) {
	for _, tc := range []struct{ yaml, want string }{
		{"openapi:\n  server: x\n", "openapi.spec is required"},
		{"openapi:\n  spec: https://example.com/openapi.yaml\n", "only be given on the command line"},
		{"openapi:\n  spec: ../openapi.yaml\n", "openapi.spec"},
		{"openapi:\n  spec: /etc/openapi.yaml\n", "must be a relative path"},
		{"openapi:\n  spec: a.yaml\n  exclude_operations: [\"health\"]\n", "must be a method and a path"},
		{"openapi:\n  spec: a.yaml\n  exclude_files: [\"[\"]\n", "exclude_files"},
		{"openapi:\n  spec: a.yaml\n  allow_remote: true\n", "allow_remote"},
	} {
		dir := t.TempDir()
		writeProjectFile(t, dir, "version: 1\n"+tc.yaml)
		if _, err := LoadProject(filepath.Join(dir, ProjectFileName)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: error %v, want %q", tc.yaml, err, tc.want)
		}
	}
}

func writeProjectFile(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ProjectFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestProjectCacheFindProjectNearestAncestor(t *testing.T) {
	c := NewProjectCache()

	path, ok, err := c.FindProject(filepath.Join(testdataConfig, "discover/sub/leaf"))
	if err != nil || !ok {
		t.Fatalf("FindProject(sub/leaf) = %q, %v, %v", path, ok, err)
	}
	wantRoot, _ := filepath.Abs(filepath.Join(testdataConfig, "discover/sonde.yaml"))
	if path != wantRoot {
		t.Errorf("FindProject(sub/leaf) = %q, want %q (nearest ancestor)", path, wantRoot)
	}

	path, ok, err = c.FindProject(filepath.Join(testdataConfig, "discover/other"))
	if err != nil || !ok {
		t.Fatalf("FindProject(other) = %q, %v, %v", path, ok, err)
	}
	wantOther, _ := filepath.Abs(filepath.Join(testdataConfig, "discover/other/sonde.yaml"))
	if path != wantOther {
		t.Errorf("FindProject(other) = %q, want %q (its own sonde.yaml, not the ancestor's)", path, wantOther)
	}
}

func TestProjectCacheFindProjectNone(t *testing.T) {
	c := NewProjectCache()
	// The filesystem root has no sonde.yaml (barring an exotic test
	// environment), so a directory with no ancestor project file at all
	// is exercised with a directory outside the repo tree.
	dir := t.TempDir()
	_, ok, err := c.FindProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("FindProject(dir with no ancestor sonde.yaml) reported found")
	}
}

func TestProjectCacheFindProjectCachesResult(t *testing.T) {
	c := NewProjectCache()
	dir := filepath.Join(testdataConfig, "discover/sub/leaf")

	first, ok1, err1 := c.FindProject(dir)
	second, ok2, err2 := c.FindProject(dir)
	if err1 != nil || err2 != nil {
		t.Fatalf("errors: %v, %v", err1, err2)
	}
	if first != second || ok1 != ok2 {
		t.Errorf("repeated FindProject(%q) = (%q,%v), (%q,%v), want identical", dir, first, ok1, second, ok2)
	}
}

// TestProjectCacheFindProjectStopsAtVCSRoot guards the trust boundary
// (docs/sonde-yaml.md, Discovery): a sonde.yaml planted above the
// repository being tested (a shared /tmp, a compromised $HOME) must not
// silently apply to a run inside it.
func TestProjectCacheFindProjectStopsAtVCSRoot(t *testing.T) {
	root := t.TempDir()
	// A "planted" sonde.yaml above the repository root.
	writeProjectFile(t, root, "version: 1\ndefaults:\n  env: planted\n")

	repo := filepath.Join(root, "repo")
	leaf := filepath.Join(repo, "sub", "leaf")
	if err := os.MkdirAll(leaf, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}

	c := NewProjectCache()
	_, ok, err := c.FindProject(leaf)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("FindProject crossed the .git boundary and found the planted sonde.yaml")
	}
}

// TestProjectCacheFindProjectVCSRootOwnFileEligible checks the boundary is
// inclusive: the VCS root directory's own sonde.yaml is still found, only
// the walk above it is stopped.
func TestProjectCacheFindProjectVCSRootOwnFileEligible(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, repo, "version: 1\n")
	leaf := filepath.Join(repo, "sub", "leaf")
	if err := os.MkdirAll(leaf, 0o750); err != nil {
		t.Fatal(err)
	}

	c := NewProjectCache()
	path, ok, err := c.FindProject(leaf)
	if err != nil || !ok {
		t.Fatalf("FindProject(leaf) = %q, %v, %v", path, ok, err)
	}
	want, _ := filepath.Abs(filepath.Join(repo, ProjectFileName))
	if path != want {
		t.Errorf("FindProject(leaf) = %q, want %q", path, want)
	}
}

// TestProjectCacheFindProjectIgnoresInsecureCandidate guards the other
// half of the trust boundary: a discovered sonde.yaml writable by group
// or others is ignored (docs/sonde-yaml.md, Discovery), and the walk
// continues to find a legitimate one further up.
func TestProjectCacheFindProjectIgnoresInsecureCandidate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("group/other write bits are not meaningful on windows")
	}
	parent := t.TempDir()
	writeProjectFile(t, parent, "version: 1\ndefaults:\n  env: parent\n")

	child := filepath.Join(parent, "child")
	if err := os.Mkdir(child, 0o750); err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, child, "version: 1\ndefaults:\n  env: child\n")
	insecure := filepath.Join(child, ProjectFileName)
	if err := os.Chmod(insecure, 0o666); err != nil { //nolint:gosec // G302: deliberately insecure, this is what the test checks is rejected
		t.Fatal(err)
	}

	leaf := filepath.Join(child, "leaf")
	if err := os.Mkdir(leaf, 0o750); err != nil {
		t.Fatal(err)
	}

	c := NewProjectCache()
	path, ok, err := c.FindProject(leaf)
	if err != nil || !ok {
		t.Fatalf("FindProject(leaf) = %q, %v, %v", path, ok, err)
	}
	want, _ := filepath.Abs(filepath.Join(parent, ProjectFileName))
	if path != want {
		t.Errorf("FindProject(leaf) = %q, want the parent's sonde.yaml %q (child's own is insecure)", path, want)
	}

	warnings := c.TakeWarnings()
	if len(warnings) == 0 {
		t.Fatal("expected a warning for the ignored insecure candidate")
	}
	if !strings.Contains(warnings[0], insecure) {
		t.Errorf("warning = %q, want it to name %q", warnings[0], insecure)
	}
	if got := c.TakeWarnings(); len(got) != 0 {
		t.Errorf("TakeWarnings did not drain: got %v", got)
	}
}

func TestProjectCacheFindProjectConcurrent(t *testing.T) {
	c := NewProjectCache()
	dirs := []string{
		filepath.Join(testdataConfig, "discover/sub/leaf"),
		filepath.Join(testdataConfig, "discover/other"),
		filepath.Join(testdataConfig, "happy"),
	}
	var wg sync.WaitGroup
	for range 50 {
		for _, dir := range dirs {
			wg.Add(1)
			go func(dir string) {
				defer wg.Done()
				if _, _, err := c.FindProject(dir); err != nil {
					t.Error(err)
				}
			}(dir)
		}
	}
	wg.Wait()
}

func TestOpenAPIExcludesFileRelativeDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	o := &OpenAPI{ExcludeFiles: []string{"legacy/*.hurl"}}
	for _, file := range []string{"legacy/a.hurl", filepath.Join(dir, "legacy/a.hurl")} {
		if !o.ExcludesFile(".", file) {
			t.Errorf("ExcludesFile(., %s) = false", file)
		}
	}
}

func TestIsRooted(t *testing.T) {
	windows := runtime.GOOS == "windows"
	for p, want := range map[string]bool{
		"secrets.env":     false,
		"dir/secrets.env": false,
		"../secrets.env":  false, // an escape, which the sandbox rejects
		"/etc/passwd":     true,
		`\etc\passwd`:     windows,
		`C:\etc\passwd`:   windows,
		"C:etc":           windows,
		`\\host\share\x`:  windows,
	} {
		if got := isRooted(p); got != want {
			t.Errorf("isRooted(%q) = %v, want %v", p, got, want)
		}
	}
}
