// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// containsMessage reports whether any diagnostic's message contains substr.
func containsMessage(diags []Diagnostic, substr string) bool {
	for _, d := range diags {
		if strings.Contains(d.Message, substr) {
			return true
		}
	}
	return false
}

//nolint:gosec // G101: "token" is the fixture's variable name, not a credential
const undefinedTokenAssert = "GET http://a/\nHTTP 200\n[Asserts]\nstatus == 200\njsonpath \"$.name\" == \"{{token}}\"\n"

// TestConfigCacheEditSondeYAMLClearsWarning is Phase 9's stated success
// criterion: editing sonde.yaml clears a stale "undefined variable"
// warning without reopening the document.
func TestConfigCacheEditSondeYAMLClearsWarning(t *testing.T) {
	dir := t.TempDir()
	sondePath := filepath.Join(dir, "sonde.yaml")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(sondePath, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("version: 1\nenvironments:\n  dev:\n    variables: {}\ndefaults:\n  env: dev\n")

	c := newTestClient(t, nil)
	c.initialize(false, dir, map[string]any{"env": "dev"})
	uri := pathToURI(filepath.Join(dir, "a.hurl"))
	diags := c.open(uri, undefinedTokenAssert)
	if !containsMessage(diags, `undefined variable "token"`) {
		t.Fatalf("diagnostics = %+v, want an undefined-variable warning", diags)
	}

	write("version: 1\nenvironments:\n  dev:\n    variables:\n      token: abc\ndefaults:\n  env: dev\n")
	c.notify("workspace/didChangeWatchedFiles", didChangeWatchedFilesParams{
		Changes: []fileEvent{{URI: pathToURI(sondePath), Type: fileChanged}},
	})
	diags = c.diagnostics(uri)
	if containsMessage(diags, "undefined variable") {
		t.Errorf("diagnostics after fixing sonde.yaml = %+v, want the warning cleared", diags)
	}
}

// TestConfigCacheDidChangeConfigurationSwitchesEnv covers the other route
// to the same success criterion: switching the active environment through
// workspace/didChangeConfiguration, with sonde.yaml untouched.
func TestConfigCacheDidChangeConfigurationSwitchesEnv(t *testing.T) {
	dir := t.TempDir()
	body := "version: 1\nenvironments:\n  dev:\n    variables: {}\n  prod:\n    variables:\n      token: abc\ndefaults:\n  env: dev\n"
	if err := os.WriteFile(filepath.Join(dir, "sonde.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	c := newTestClient(t, nil)
	c.initialize(false, dir, map[string]any{"env": "dev"})
	uri := pathToURI(filepath.Join(dir, "a.hurl"))
	diags := c.open(uri, undefinedTokenAssert)
	if !containsMessage(diags, `undefined variable "token"`) {
		t.Fatalf("diagnostics under dev = %+v, want an undefined-variable warning", diags)
	}

	c.notify("workspace/didChangeConfiguration", didChangeConfigurationParams{Settings: json.RawMessage(`{"sonde":{"env":"prod"}}`)})
	diags = c.diagnostics(uri)
	if containsMessage(diags, "undefined variable") {
		t.Errorf("diagnostics under prod = %+v, want token defined there", diags)
	}
}

// TestRegisterWatchersOnDynamicRegistration checks the initial
// registration a client offering dynamic registration gets right after
// initialized, before any project has loaded.
func TestRegisterWatchersOnDynamicRegistration(t *testing.T) {
	c := newTestClient(t, nil)
	c.initialize(true, "", nil)
	m := c.next("client/registerCapability", nil)
	var p registrationParams
	if err := json.Unmarshal(m.Params, &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Registrations) != 1 || p.Registrations[0].Method != "workspace/didChangeWatchedFiles" {
		t.Fatalf("registrations = %+v", p.Registrations)
	}
	var opts didChangeWatchedFilesRegistrationOptions
	raw, _ := json.Marshal(p.Registrations[0].RegisterOptions)
	if err := json.Unmarshal(raw, &opts); err != nil {
		t.Fatal(err)
	}
	if len(opts.Watchers) != 1 || opts.Watchers[0].GlobPattern != "**/sonde.yaml" {
		t.Errorf("watchers = %+v", opts.Watchers)
	}
}

// TestConfigCacheFolderBoundary checks that a sonde.yaml above the
// workspace folder containing a document is never read.
func TestConfigCacheFolderBoundary(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	body := "version: 1\nenvironments:\n  dev:\n    variables:\n      token: abc\ndefaults:\n  env: dev\n"
	if err := os.WriteFile(filepath.Join(root, "sonde.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	c := newTestClient(t, nil)
	c.initialize(false, sub, map[string]any{"env": "dev"})
	uri := pathToURI(filepath.Join(sub, "a.hurl"))
	diags := c.open(uri, undefinedTokenAssert)
	if !containsMessage(diags, "no sonde.yaml environment selected") {
		t.Errorf("diagnostics = %+v, want token undefined: the sonde.yaml above the folder must not apply", diags)
	}
}

// TestConfigCacheSecretsNeverLeak checks that a secrets_files value never
// reaches a diagnostic message, and that the cache itself holds only the
// secret's name and file, never its value.
func TestConfigCacheSecretsNeverLeak(t *testing.T) {
	dir := t.TempDir()
	const secretValue = "sk_live_do_not_leak_9f3c" //nolint:gosec // G101: test fixture value, never a real credential
	if err := os.WriteFile(filepath.Join(dir, "secrets.env"), []byte("apiKey="+secretValue+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	body := "version: 1\nenvironments:\n  dev:\n    secrets_files:\n      - secrets.env\ndefaults:\n  env: dev\n"
	if err := os.WriteFile(filepath.Join(dir, "sonde.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	c := newTestClient(t, nil)
	c.initialize(false, dir, map[string]any{"env": "dev"})
	uri := pathToURI(filepath.Join(dir, "a.hurl"))
	diags := c.open(uri, "GET http://a/\nHTTP 200\n[Asserts]\nstatus == 200\njsonpath \"$.name\" == \"{{apiKey}}\"\n")
	if containsMessage(diags, `undefined variable "apiKey"`) {
		t.Fatalf("diagnostics = %+v, want apiKey recognized as a secret", diags)
	}
	for _, d := range diags {
		if strings.Contains(d.Message, secretValue) {
			t.Fatalf("a diagnostic leaked the secret value: %+v", d)
		}
	}

	// The cache's own representation: varSource has no value field, only
	// kind and file, so nothing beyond the name can flow from here either.
	s, err := NewServer(Options{})
	if err != nil {
		t.Fatal(err)
	}
	s.folders = []string{dir}
	d := newDocument(uri, 1, "GET http://a/\nHTTP 200\n", true)
	pv := s.config.variables(d)
	src, ok := pv.names["apiKey"]
	if !ok || src.kind != srcSecretsFile || src.file != "secrets.env" {
		t.Fatalf("apiKey source = %+v, ok=%v, want srcSecretsFile from secrets.env", src, ok)
	}
}

// TestConfigCacheProcessEnvOutranksSondeYAML: a name defined both in
// sonde.yaml and by SONDE_VARIABLE_* reports the process environment, the
// source a run would use.
func TestConfigCacheProcessEnvOutranksSondeYAML(t *testing.T) {
	dir := t.TempDir()
	body := "version: 1\nenvironments:\n  dev:\n    variables:\n      host: a\n      port: 1\ndefaults:\n  env: dev\n"
	if err := os.WriteFile(filepath.Join(dir, "sonde.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(Options{Environ: map[string]string{"SONDE_VARIABLE_host": "b"}})
	if err != nil {
		t.Fatal(err)
	}
	s.folders = []string{dir}
	pv := s.config.variables(newDocument(pathToURI(filepath.Join(dir, "a.hurl")), 1, "", true))
	if pv.err != nil {
		t.Fatal(pv.err)
	}
	if got := pv.names["host"].kind; got != srcProcessEnv {
		t.Errorf("host source = %v, want srcProcessEnv", got)
	}
	if got := pv.names["port"].kind; got != srcEnvironment {
		t.Errorf("port source = %v, want srcEnvironment", got)
	}
}

// TestConfigCacheProcessSecretOutranksSondeYAML checks HURL_SECRET_*/
// SONDE_SECRET_* names are recognized (M3 in the phase 9 LSP review), with
// srcProcessSecret and never a value, and that they outrank sonde.yaml the
// same way a process variable does.
func TestConfigCacheProcessSecretOutranksSondeYAML(t *testing.T) {
	dir := t.TempDir()
	body := "version: 1\nenvironments:\n  dev:\n    variables:\n      apiKey: a\ndefaults:\n  env: dev\n"
	if err := os.WriteFile(filepath.Join(dir, "sonde.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(Options{Environ: map[string]string{"SONDE_SECRET_apiKey": "shh", "SONDE_SECRET_other": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	s.folders = []string{dir}
	pv := s.config.variables(newDocument(pathToURI(filepath.Join(dir, "a.hurl")), 1, "", true))
	if pv.err != nil {
		t.Fatal(pv.err)
	}
	if got := pv.names["apiKey"].kind; got != srcProcessSecret {
		t.Errorf("apiKey source = %v, want srcProcessSecret", got)
	}
	if got := pv.names["other"].kind; got != srcProcessSecret {
		t.Errorf("other source = %v, want srcProcessSecret", got)
	}
}

// TestConfigCacheNoFolderSkipsConfig checks a document outside every
// workspace folder gets no configuration at all (M1): not even a
// sonde.yaml sitting right next to it on disk is read.
func TestConfigCacheNoFolderSkipsConfig(t *testing.T) {
	dir := t.TempDir()
	body := "version: 1\nenvironments:\n  dev:\n    variables:\n      token: abc\ndefaults:\n  env: dev\n"
	if err := os.WriteFile(filepath.Join(dir, "sonde.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := NewServer(Options{})
	if err != nil {
		t.Fatal(err)
	}
	// s.folders stays empty: single-file mode, no workspace folder at all.
	d := newDocument(pathToURI(filepath.Join(dir, "a.hurl")), 1, "GET http://a/\nHTTP 200\n", true)
	pv := s.config.variables(d)
	if !pv.noFolder {
		t.Error("pv.noFolder = false, want true with no workspace folder")
	}
	if pv.project != "" {
		t.Errorf("pv.project = %q, want \"\": the sonde.yaml next to the document must not be read", pv.project)
	}
}

// TestConfigCacheStopsAtVCSRoot checks discovery stops at a VCS root (a
// directory containing .git), matching the CLI (config.ProjectCache), even
// when a sonde.yaml above it is still within the workspace folder.
func TestConfigCacheStopsAtVCSRoot(t *testing.T) {
	root := t.TempDir()
	body := "version: 1\nenvironments:\n  dev:\n    variables:\n      token: abc\ndefaults:\n  env: dev\n"
	if err := os.WriteFile(filepath.Join(root, "sonde.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}

	c := newTestClient(t, nil)
	c.initialize(false, root, map[string]any{"env": "dev"})
	uri := pathToURI(filepath.Join(repo, "a.hurl"))
	diags := c.open(uri, undefinedTokenAssert)
	if !containsMessage(diags, "no sonde.yaml environment selected") {
		t.Errorf("diagnostics = %+v, want discovery to stop at repo's .git and not see root/sonde.yaml", diags)
	}
}

// TestConfigCacheRejectsSymlinkedSondeYAML checks a sonde.yaml that is
// itself a symbolic link leaving the workspace folder is rejected, even
// though its nominal path looks like it is inside (M1).
func TestConfigCacheRejectsSymlinkedSondeYAML(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	body := "version: 1\nenvironments:\n  dev:\n    variables:\n      token: abc\ndefaults:\n  env: dev\n"
	realYAML := filepath.Join(outside, "real-sonde.yaml")
	if err := os.WriteFile(realYAML, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realYAML, filepath.Join(ws, "sonde.yaml")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	c := newTestClient(t, nil)
	c.initialize(false, ws, map[string]any{"env": "dev"})
	uri := pathToURI(filepath.Join(ws, "a.hurl"))
	diags := c.open(uri, undefinedTokenAssert)
	if !containsMessage(diags, "no sonde.yaml environment selected") {
		t.Errorf("diagnostics = %+v, want a symlinked sonde.yaml leaving the folder rejected", diags)
	}
}

// TestConfigCacheIgnoresInsecureCandidate checks a world-writable
// sonde.yaml is ignored, the same way the CLI's discovery does (reused via
// config.ProjectCache, not reimplemented), and that the reason surfaces as
// a diagnostic (kongming checkpoint, phase 9 Should-fix 1) instead of
// leaving the user with an unexplained "no sonde.yaml environment
// selected".
func TestConfigCacheIgnoresInsecureCandidate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("group/other write bits are not meaningful on windows")
	}
	dir := t.TempDir()
	body := "version: 1\nenvironments:\n  dev:\n    variables:\n      token: abc\ndefaults:\n  env: dev\n"
	path := filepath.Join(dir, "sonde.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o666); err != nil { //nolint:gosec // G302: deliberately insecure, this is what the test checks is rejected
		t.Fatal(err)
	}

	c := newTestClient(t, nil)
	c.initialize(false, dir, map[string]any{"env": "dev"})
	uri := pathToURI(filepath.Join(dir, "a.hurl"))
	diags := c.open(uri, undefinedTokenAssert)
	if !containsMessage(diags, "no sonde.yaml environment selected") {
		t.Errorf("diagnostics = %+v, want a world-writable sonde.yaml ignored like the CLI does", diags)
	}
	if !containsMessage(diags, "ignored (not owned by the current user, or writable by group or others)") {
		t.Errorf("diagnostics = %+v, want the skipped-candidate reason surfaced", diags)
	}

	// A second lookup for the same document must not accumulate a second
	// copy of the warning (drained per lookup, not left piled up).
	diags = c.change(uri, 2, undefinedTokenAssert)
	n := 0
	for _, d := range diags {
		if strings.Contains(d.Message, "ignored (not owned by the current user, or writable by group or others)") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("saw the insecure-candidate warning %d times, want exactly 1", n)
	}
}

// TestConfigCacheDiscoveryNoticesNewSondeYAML checks a sonde.yaml created
// after a document was opened is picked up on the next change, with no
// workspace/didChangeWatchedFiles notification at all (M6): discovery
// itself revalidates by stat, not just an already-found project's content.
func TestConfigCacheDiscoveryNoticesNewSondeYAML(t *testing.T) {
	dir := t.TempDir()
	c := newTestClient(t, nil)
	c.initialize(false, dir, map[string]any{"env": "dev"})
	uri := pathToURI(filepath.Join(dir, "a.hurl"))
	diags := c.open(uri, undefinedTokenAssert)
	if !containsMessage(diags, "no sonde.yaml environment selected") {
		t.Fatalf("diagnostics = %+v, want no project yet", diags)
	}

	body := "version: 1\nenvironments:\n  dev:\n    variables:\n      token: abc\ndefaults:\n  env: dev\n"
	if err := os.WriteFile(filepath.Join(dir, "sonde.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	diags = c.change(uri, 2, undefinedTokenAssert)
	if containsMessage(diags, "undefined variable") {
		t.Errorf("diagnostics after creating sonde.yaml = %+v, want it picked up without a watch event", diags)
	}
}

// TestConfigCacheDiscoveryNoticesDeletedSondeYAML is the reverse of
// TestConfigCacheDiscoveryNoticesNewSondeYAML: removing sonde.yaml is
// noticed on the next change too, falling back to no project.
func TestConfigCacheDiscoveryNoticesDeletedSondeYAML(t *testing.T) {
	dir := t.TempDir()
	body := "version: 1\nenvironments:\n  dev:\n    variables:\n      token: abc\ndefaults:\n  env: dev\n"
	path := filepath.Join(dir, "sonde.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	c := newTestClient(t, nil)
	c.initialize(false, dir, map[string]any{"env": "dev"})
	uri := pathToURI(filepath.Join(dir, "a.hurl"))
	diags := c.open(uri, undefinedTokenAssert)
	if containsMessage(diags, "undefined variable") {
		t.Fatalf("diagnostics = %+v, want token defined initially", diags)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	diags = c.change(uri, 2, undefinedTokenAssert)
	if !containsMessage(diags, "no sonde.yaml environment selected") {
		t.Errorf("diagnostics after deleting sonde.yaml = %+v, want it noticed without a watch event", diags)
	}
}

// TestConfigCacheEmptyEnvSettingFallsBackToDefault checks an explicit
// sonde.env:"" is treated as unset, falling back to SONDE_ENV then
// defaults.env, rather than disabling the environment outright (L3): the
// VS Code UI stores "" when a user clears the setting.
func TestConfigCacheEmptyEnvSettingFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	body := "version: 1\nenvironments:\n  dev:\n    variables:\n      token: abc\ndefaults:\n  env: dev\n"
	if err := os.WriteFile(filepath.Join(dir, "sonde.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	c := newTestClient(t, nil)
	c.initialize(false, dir, map[string]any{"env": ""})
	uri := pathToURI(filepath.Join(dir, "a.hurl"))
	diags := c.open(uri, undefinedTokenAssert)
	if containsMessage(diags, "undefined variable") {
		t.Errorf(`diagnostics with env:"" = %+v, want a fallback to defaults.env, not a disabled environment`, diags)
	}
}

// TestFolderContainingRoot checks "/" (or a drive root) contains
// everything, since it already ends in a separator and the old prefix check
// ("/" + separator = "//") never matched anything (L16).
func TestFolderContainingRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.VolumeName(dir) + string(filepath.Separator)
	s := &Server{folders: []string{root}}
	folder, ok := s.folderContaining(dir)
	if !ok || folder != root {
		t.Errorf("folderContaining(%q) = %q, %v, want %q, true", dir, folder, ok, root)
	}
}

// TestExtraVariables checks the "extraVariables" setting: names the client
// defines are not undefined, at initialize and after a change.
func TestExtraVariables(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sonde.yaml"), []byte("version: 1\nenvironments:\n  dev:\n    variables: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newTestClient(t, nil)
	c.initialize(false, dir, map[string]any{"extraVariables": []string{"token"}})
	uri := pathToURI(filepath.Join(dir, "a.hurl"))
	if diags := c.open(uri, undefinedTokenAssert); containsMessage(diags, "undefined variable") {
		t.Fatalf("diagnostics = %+v, want token defined by the client", diags)
	}
	c.notify("workspace/didChangeConfiguration", didChangeConfigurationParams{Settings: json.RawMessage(`{"sonde":{"extraVariables":[]}}`)})
	if diags := c.diagnostics(uri); !containsMessage(diags, `undefined variable "token"`) {
		t.Errorf("diagnostics = %+v, want token undefined again", diags)
	}
}
