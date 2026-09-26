// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

func writeEnvFiles(t *testing.T, dir, env, private string) string {
	t.Helper()
	path := filepath.Join(dir, "http-client.env.json")
	if err := os.WriteFile(path, []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	if private != "" {
		p := filepath.Join(dir, "http-client.private.env.json")
		if err := os.WriteFile(p, []byte(private), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestImportWithEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := writeEnvFiles(t, dir,
		`{"$shared": {"scheme": "https"}, "dev": {"host": "dev.test"}, "prod": {"host": "prod.test", "port": 443}}`,
		`{"dev": {"token": "x"}, "prod": {"token": "y"}}`)

	src := "GET https://a.test/x\n"
	out, err := Import("req", []byte(src), syntax.DialectHurl, Options{EnvFiles: []string{envFile}})
	if err != nil {
		t.Fatal(err)
	}
	if out.ProjectYAML == nil {
		t.Fatal("expected a sonde.yaml skeleton")
	}
	doc := string(out.ProjectYAML)
	for _, want := range []string{"dev:", "prod:", "host: dev.test", "host: prod.test", "port: \"443\""} {
		if !strings.Contains(doc, want) {
			t.Errorf("sonde.yaml missing %q:\n%s", want, doc)
		}
	}
	if strings.Contains(doc, "\"x\"") || strings.Contains(doc, "\"y\"") {
		t.Error("private env values must never be written")
	}

	var secretPaths []string
	for _, f := range out.Extra {
		secretPaths = append(secretPaths, f.Path)
	}
	if !contains(secretPaths, "secrets/dev.secrets") || !contains(secretPaths, "secrets/prod.secrets") {
		t.Errorf("secret stub paths = %v", secretPaths)
	}
	for _, f := range out.Extra {
		if !f.Keep {
			t.Errorf("%s should be Keep=true", f.Path)
		}
		if strings.TrimSpace(string(f.Data)) != "token=" {
			t.Errorf("%s should hold no value: %q", f.Path, f.Data)
		}
	}

	var secretWarnings int
	for _, w := range out.Warnings {
		if w.Kind == convert.WarnSecret {
			secretWarnings++
		}
	}
	if secretWarnings != 2 {
		t.Errorf("secret warnings = %d, want 2", secretWarnings)
	}
}

func TestImportWithEnvFileNoPrivateSibling(t *testing.T) {
	dir := t.TempDir()
	envFile := writeEnvFiles(t, dir, `{"dev": {"host": "dev.test"}}`, "")
	out, err := Import("req", []byte("GET https://a.test/x\n"), syntax.DialectHurl, Options{EnvFiles: []string{envFile}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Extra) != 0 {
		t.Errorf("Extra = %v, want none", out.Extra)
	}
}

func TestImportWithEnvFileMissing(t *testing.T) {
	_, err := Import("req", []byte("GET https://a.test/x\n"), syntax.DialectHurl,
		Options{EnvFiles: []string{"/does/not/exist.env.json"}})
	if err == nil {
		t.Fatal("expected an error for a missing --env-file")
	}
}

func TestImportWithEnvFileInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	envFile := writeEnvFiles(t, dir, `not json`, "")
	_, err := Import("req", []byte("GET https://a.test/x\n"), syntax.DialectHurl, Options{EnvFiles: []string{envFile}})
	if err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}

// TestImportEnvFileRefusesPrivateSibling checks M12: passing a
// "*.private.env.json" file directly to --env-file is refused with a clear
// error, rather than its secret values being written into sonde.yaml as if
// they were public.
func TestImportEnvFileRefusesPrivateSibling(t *testing.T) {
	dir := t.TempDir()
	writeEnvFiles(t, dir, `{"dev": {"host": "dev.test"}}`, `{"dev": {"token": "s3cret"}}`)
	privatePath := filepath.Join(dir, "http-client.private.env.json")
	_, err := Import("req", []byte("GET https://a.test/x\n"), syntax.DialectHurl, Options{EnvFiles: []string{privatePath}})
	if err == nil {
		t.Fatal("expected an error for a private env file passed directly")
	}
	if !strings.Contains(err.Error(), "private") {
		t.Errorf("error should mention it is a private file: %v", err)
	}
}

// TestImportEnvFileSeedsFileVariables checks M7: a non-default environment
// is seeded with the file's own "@var = value" variables, with its own
// values winning on a name collision.
func TestImportEnvFileSeedsFileVariables(t *testing.T) {
	dir := t.TempDir()
	envFile := writeEnvFiles(t, dir, `{"dev": {"host": "dev.test"}}`, "")
	src := "@host = default.test\n@scheme = https\nGET https://a.test/x\n"
	out, err := Import("req", []byte(src), syntax.DialectHurl, Options{EnvFiles: []string{envFile}})
	if err != nil {
		t.Fatal(err)
	}
	doc := string(out.ProjectYAML)
	_, afterDev, ok := strings.Cut(doc, "dev:\n")
	if !ok {
		t.Fatalf("sonde.yaml has no dev environment:\n%s", doc)
	}
	devBlock, _, _ := strings.Cut(afterDev, "defaults:")
	if !strings.Contains(devBlock, "scheme: https") {
		t.Errorf("dev environment should inherit the file variable \"scheme\":\n%s", devBlock)
	}
	if !strings.Contains(devBlock, "host: dev.test") {
		t.Errorf("dev's own \"host\" should win over the file variable default:\n%s", devBlock)
	}
	if strings.Contains(devBlock, "host: default.test") {
		t.Errorf("the file variable default should not leak into dev once overridden:\n%s", devBlock)
	}
}

// TestImportEnvFileFileVariableResolvesPerEnvironment checks S2: a file
// variable whose value references a name defined only in the env file
// (e.g. "host") must resolve using *each* environment's own value for it,
// not stay a literal, unresolved "{{host}}" copied into every environment.
func TestImportEnvFileFileVariableResolvesPerEnvironment(t *testing.T) {
	dir := t.TempDir()
	envFile := writeEnvFiles(t, dir, `{"dev": {"host": "dev.test"}, "prod": {"host": "prod.test"}}`, "")
	src := "@base = https://{{host}}/api\nGET https://a.test/x\n"
	out, err := Import("req", []byte(src), syntax.DialectHurl, Options{EnvFiles: []string{envFile}})
	if err != nil {
		t.Fatal(err)
	}
	doc := string(out.ProjectYAML)
	// The "default" environment has no env-file context to resolve "host"
	// against, so it legitimately stays literal (unchanged, pre-existing
	// behavior): only "dev" and "prod" are checked here.
	for env, want := range map[string]string{"dev": "https://dev.test/api", "prod": "https://prod.test/api"} {
		_, after, ok := strings.Cut(doc, env+":\n")
		if !ok {
			t.Fatalf("sonde.yaml has no %s environment:\n%s", env, doc)
		}
		block, _, _ := strings.Cut(after, "defaults:")
		if strings.Contains(block, "{{host}}") {
			t.Errorf("%s: a file variable should resolve per environment, not stay literal:\n%s", env, block)
		}
		if !strings.Contains(block, "base: "+want) {
			t.Errorf("%s should resolve base using its own host:\n%s", env, block)
		}
	}
}

// TestImportEnvFilePrivateSharedFoldsIntoEveryEnvironment checks M12's
// second half: a private "$shared" secret applies to every environment,
// including one with no private entry of its own.
func TestImportEnvFilePrivateSharedFoldsIntoEveryEnvironment(t *testing.T) {
	dir := t.TempDir()
	envFile := writeEnvFiles(t, dir,
		`{"dev": {"host": "dev.test"}, "prod": {"host": "prod.test"}}`,
		`{"$shared": {"apiKey": "s3cret"}, "dev": {"token": "d"}}`)
	out, err := Import("req", []byte("GET https://a.test/x\n"), syntax.DialectHurl, Options{EnvFiles: []string{envFile}})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, f := range out.Extra {
		paths[f.Path] = true
		if strings.Contains(f.Path, "dev") && !strings.Contains(string(f.Data), "apiKey=") {
			t.Errorf("dev stub should include the shared secret name: %s", f.Data)
		}
		if strings.Contains(f.Path, "prod") && !strings.Contains(string(f.Data), "apiKey=") {
			t.Errorf("prod stub (no private entry of its own) should still get the shared secret name: %s", f.Data)
		}
	}
	if !paths["secrets/dev.secrets"] || !paths["secrets/prod.secrets"] {
		t.Fatalf("Extra paths = %v", paths)
	}
}
