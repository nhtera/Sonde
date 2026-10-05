// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package envsvc is the project's environments: their variables and where
// each comes from, edits written to that source (sonde.yaml, a variables
// file, a secrets file), Mark secret, and session overrides. It is also the
// single owner of a run's overrides: every value that changes a run beyond
// the project and its files (the app's settings, session overrides, the
// mock, and what runplan reports from the CLI's config file and HURL_*/
// SONDE_* variables); the overrides chip, Copy as and runs all read it.
// Secret values never reach the page.
package envsvc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/credential"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/runplan"
	"github.com/nhtera/sonde/internal/sandbox"
	"github.com/nhtera/sonde/internal/value"
)

// TopicChanged is sent after an edit or an override change.
const TopicChanged = "env:changed"

// Var is a variable of an environment. A secret has no Value.
type Var struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Type   string `json:"type"`
	Source string `json:"source"` // "sonde.yaml" or the file, project-relative
	Secret bool   `json:"secret"`
}

// Env is an environment of the project.
type Env struct {
	Name      string `json:"name"`
	Default   bool   `json:"default"`
	Variables []Var  `json:"variables"`
	// SecretsFile is where a new secret of env goes ("" when env's
	// name cannot name one).
	SecretsFile string `json:"secretsFile"`
	// Error is why env does not resolve (a missing file…).
	Error string `json:"error,omitempty"`
}

// Project is the project's environments.
type Project struct {
	// Config is sonde.yaml's project path; "" when the project has none.
	Config string `json:"config"`
	Envs   []Env  `json:"envs"`
}

// Override is a value that changes a run beyond the project.
type Override struct {
	Name string `json:"name"`
	// Source is "settings", "session", "mock", "environment" (HURL_*/
	// SONDE_*) or "config file".
	Source string `json:"source"`
	// Origin is where it came from (the variable or the config file).
	Origin string `json:"origin,omitempty"`
	// Flag is the CLI flag that reproduces it, e.g. "--max-time".
	Flag string `json:"flag"`
}

// Overrides is the overrides chip: its count and items.
type Overrides struct {
	Count int        `json:"count"`
	Items []Override `json:"items"`
}

// Envs is the environment service's core.
type Envs struct {
	emit     emit.Emitter
	config   *sandbox.Root // the app's config folder (edit journal)
	project  func() *sandbox.Root
	env      config.Env
	version  string
	settings func(inv *runplan.Invocation) // the settings' flags
	// KeepCookies reports whether Settings › Keep cookies is on: runs then
	// read and write each file's kept jar (-b and -c), an override too.
	KeepCookies func() bool

	editMu    sync.Mutex // one project edit at a time, load to journal removal
	mu        sync.Mutex
	session   map[string]string // session overrides: name -> value text
	mock      string            // the mock's URL while it runs
	recovered string            // the project whose journal was checked
}

// New returns the environment service.
func New(e emit.Emitter, appConfig *sandbox.Root, project func() *sandbox.Root, env config.Env, version string, settings func(*runplan.Invocation)) *Envs {
	return &Envs{emit: e, config: appConfig, project: project, env: env, version: version, settings: settings, session: map[string]string{}}
}

// load returns the open project's sonde.yaml, recovering an unfinished
// edit first; nil when the project has none.
func (e *Envs) load() (*sandbox.Root, *config.Project, error) {
	root := e.project()
	if root == nil {
		return nil, nil, apperr.New(apperr.NotFound, "no project is open")
	}
	e.mu.Lock()
	if e.recovered != root.Dir() {
		recoverEdit(e.config, root)
		e.recovered = root.Dir()
	}
	e.mu.Unlock()
	for _, name := range []string{"sonde.yaml", "sonde.yml"} {
		if _, err := root.Stat(name); err == nil {
			p, err := config.LoadProject(filepath.Join(root.Dir(), name))
			return root, p, err
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, nil, err
		}
	}
	return root, nil, nil
}

// List returns the project's environments and their variables.
func (e *Envs) List() (*Project, error) {
	_, p, err := e.load()
	if err != nil {
		return nil, err
	}
	out := &Project{Envs: []Env{}}
	if p == nil {
		return out, nil
	}
	out.Config = filepath.Base(p.Path)
	names := make([]string, 0, len(p.Environments))
	for n := range p.Environments {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		env := Env{Name: n, Default: p.Defaults.Env == n, Variables: []Var{}}
		env.SecretsFile, _ = p.SecretsFileFor(n)
		sources, err := p.VariableNames(n)
		if err != nil {
			env.Error = err.Error()
		}
		vars, _, rerr := p.Resolve(n)
		if rerr != nil && env.Error == "" {
			env.Error = rerr.Error()
		}
		for name, src := range sources {
			v := Var{Name: name, Source: out.Config, Secret: src.Secret}
			if src.File != "" {
				v.Source = src.File
			}
			if !src.Secret {
				if val, ok := vars[name]; ok {
					v.Value, v.Type = value.Display(val), val.Kind().String()
				}
			}
			env.Variables = append(env.Variables, v)
		}
		slices.SortFunc(env.Variables, func(a, b Var) int { return strings.Compare(a.Name, b.Name) })
		out.Envs = append(out.Envs, env)
	}
	return out, nil
}

// SetVariable sets (or adds) variable name of env in its effective source.
// raw is its JSON value: a string, number, boolean or null keeps its type.
func (e *Envs) SetVariable(env, name string, raw json.RawMessage) error {
	v, err := value.DecodeJSON(string(raw))
	if err != nil {
		return apperr.New(apperr.Invalid, "not a JSON value: "+err.Error())
	}
	if s, ok := v.(value.String); ok && strings.ContainsAny(string(s), "\r\n") {
		return apperr.New(apperr.Invalid, "a variable is one line")
	}
	return e.edit(func(p *config.Project) ([]config.FileEdit, error) { return p.SetVariable(env, name, v) })
}

// RemoveVariable removes name from every source of env.
func (e *Envs) RemoveVariable(env, name string) error {
	return e.edit(func(p *config.Project) ([]config.FileEdit, error) { return p.RemoveVariable(env, name) })
}

// Names lists the variables and secrets of env.
func (e *Envs) Names(env string) map[string]bool {
	out := map[string]bool{}
	p, err := e.List()
	if err != nil {
		return out
	}
	for _, en := range p.Envs {
		if en.Name == env {
			for _, v := range en.Variables {
				out[v.Name] = true
			}
		}
	}
	return out
}

// SetSecret sets (or adds) secret name of env in its secrets file (0600:
// the env's last one, or secrets/<env>.secrets, added to secrets_files);
// the value is never written to sonde.yaml, and a variable of that name
// there goes.
func (e *Envs) SetSecret(env, name, secret string) error {
	if strings.ContainsAny(secret, "\r\n") {
		return apperr.New(apperr.Invalid, "a secret is one line")
	}
	return e.edit(func(p *config.Project) ([]config.FileEdit, error) { return p.SetSecret(env, name, secret) })
}

// MarkSecret moves variable name of env to a secrets file (0600): the
// env's last one, or secrets/<env>.secrets, added to secrets_files.
func (e *Envs) MarkSecret(env, name string) error {
	return e.edit(func(p *config.Project) ([]config.FileEdit, error) {
		vars, _, err := p.Resolve(env)
		if err != nil {
			return nil, err
		}
		v, ok := vars[name]
		if !ok {
			return nil, apperr.New(apperr.NotFound, name+" is not a variable of "+env)
		}
		return p.SetSecret(env, name, value.Display(v))
	})
}

// edit computes edits (validated by config against every environment),
// then writes them as one transaction.
func (e *Envs) edit(fn func(*config.Project) ([]config.FileEdit, error)) error {
	e.editMu.Lock()
	defer e.editMu.Unlock()
	root, p, err := e.load()
	if err != nil {
		return err
	}
	if p == nil {
		return apperr.New(apperr.NotFound, "the project has no sonde.yaml")
	}
	edits, err := fn(p)
	if err != nil {
		if errors.Is(err, config.ErrEditByHand) {
			return apperr.Wrap(apperr.Invalid, err)
		}
		var ae *apperr.Error
		if errors.As(err, &ae) {
			return err
		}
		return apperr.Wrap(apperr.Invalid, err)
	}
	for _, ed := range edits {
		rel, err := filepath.Rel(root.Dir(), ed.Path)
		if err != nil || strings.HasPrefix(rel, "..") {
			return apperr.New(apperr.Denied, "an edit outside the project")
		}
		for _, part := range strings.Split(filepath.ToSlash(filepath.Dir(rel)), "/") {
			if strings.HasPrefix(part, ".") && part != "." {
				return apperr.New(apperr.Denied, "the app does not write in dot folders: "+rel)
			}
		}
	}
	if err := apply(e.config, root, edits); err != nil {
		return err
	}
	e.emit.Emit(TopicChanged, nil)
	// A secrets file written is kept out of git; failing that is not the
	// edit failing (the value is written), and commits leave it out anyway.
	for _, ed := range edits {
		rel, err := filepath.Rel(root.Dir(), ed.Path)
		if err != nil || !e.IsSecretFile(filepath.ToSlash(rel)) {
			continue
		}
		if added, err := ignoreSecrets(root, filepath.ToSlash(rel)); err == nil && added {
			e.emit.Emit(TopicIgnored, map[string]string{"line": secretsPattern, "file": filepath.ToSlash(rel)})
		}
		break
	}
	return nil
}

// SetOverride sets session override name (a --variable for this session's
// runs); an empty name clears none. A name the project or the environment
// holds as a secret is refused, as the CLI refuses it.
func (e *Envs) SetOverride(env, name, text string) error {
	if name == "" || strings.ContainsAny(name, "= \t\r\n") {
		return apperr.New(apperr.Invalid, "not a variable name: "+name)
	}
	if strings.ContainsAny(text, "\r\n") {
		return apperr.New(apperr.Invalid, "a variable is one line")
	}
	if _, p, err := e.load(); err == nil && p != nil && env != "" {
		if _, secrets, err := p.Resolve(env); err == nil {
			if _, ok := secrets[name]; ok {
				return apperr.New(apperr.Invalid, config.CheckNoClash(map[string]value.Value{name: value.String(text)}, map[string]string{name: ""}).Error())
			}
		}
	}
	if _, ok := e.env.SecretEnvVars()[name]; ok {
		return apperr.New(apperr.Invalid, config.CheckNoClash(map[string]value.Value{name: value.String(text)}, map[string]string{name: ""}).Error())
	}
	e.mu.Lock()
	e.session[name] = text
	e.mu.Unlock()
	e.emit.Emit(TopicChanged, nil)
	return nil
}

// IsSecretFile reports whether rel (project path) is a secrets file of the
// project's sonde.yaml, of any environment.
func (e *Envs) IsSecretFile(rel string) bool {
	_, p, err := e.load()
	if err != nil || p == nil {
		return false
	}
	for _, env := range p.Environments {
		for _, f := range env.SecretsFiles {
			if filepath.ToSlash(filepath.Clean(f)) == rel {
				return true
			}
		}
	}
	return false
}

// Opened resets what belongs to the previous project (session overrides,
// the mock) and rolls back an unfinished edit of the new one, before
// anything reads it.
func (e *Envs) Opened() {
	e.mu.Lock()
	e.session, e.mock, e.recovered = map[string]string{}, "", ""
	e.mu.Unlock()
	_, _, _ = e.load()
	e.emit.Emit(TopicChanged, nil)
}

// SessionOverrides returns the session overrides (name -> value).
func (e *Envs) SessionOverrides() map[string]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[string]string, len(e.session)+1)
	for k, v := range e.session {
		out[k] = v
	}
	if e.mock != "" {
		out["base_url"] = e.mock
	}
	return out
}

// RemoveOverride removes session override name.
func (e *Envs) RemoveOverride(name string) {
	e.mu.Lock()
	delete(e.session, name)
	e.mu.Unlock()
	e.emit.Emit(TopicChanged, nil)
}

// SetMock sets (url) or clears ("") the mock's base_url override.
func (e *Envs) SetMock(url string) {
	e.mu.Lock()
	e.mock = url
	e.mu.Unlock()
	e.emit.Emit(TopicChanged, nil)
}

// Extend adds the overrides to a run's invocation: the settings' flags,
// the mock's base_url and the session overrides (as --variable).
func (e *Envs) Extend(inv *runplan.Invocation) {
	if e.settings != nil {
		e.settings(inv)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if inv.Set == nil {
		inv.Set = map[string]bool{}
	}
	names := make([]string, 0, len(e.session))
	for n := range e.session {
		names = append(names, n)
	}
	sort.Strings(names)
	if e.mock != "" {
		inv.Variables = append(inv.Variables, "base_url="+e.mock)
		inv.Set["variable"] = true
	}
	for _, n := range names {
		inv.Variables = append(inv.Variables, n+"="+e.session[n])
		inv.Set["variable"] = true
	}
}

// Command adds to a command for CI (Copy as › sonde) the variables of the
// app's environment (SONDE_VARIABLE_…), so that it reproduces the run: a
// run reads them from the environment (a --variable would rank above a
// data row), a command may run where they are not set. Those that look
// like credentials are not shown: their names are returned, for the user
// to set them in CI.
func (e *Envs) Command(inv *runplan.Invocation) (held []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if inv.Set == nil {
		inv.Set = map[string]bool{}
	}
	for _, v := range e.envVariables() {
		name, val, _ := strings.Cut(v, "=")
		if credential.Likely(name, val) {
			held = append(held, name)
			continue
		}
		inv.Variables = append(inv.Variables, v)
		inv.Set["variable"] = true
	}
	return held
}

// envVariables are the variables the app's environment sets, as
// name=value, in name order; those a session override or the mock sets
// are left out.
func (e *Envs) envVariables() []string {
	var out []string
	for name, val := range e.env.VariableEnvVars() {
		if _, overridden := e.session[name]; overridden || (e.mock != "" && name == "base_url") {
			continue
		}
		out = append(out, name+"="+val)
	}
	sort.Strings(out)
	return out
}

// Overrides lists every override of a run: the app's own (Extend) and
// what runplan reports from the CLI's config file and environment.
func (e *Envs) Overrides() Overrides {
	var items []Override
	var inv runplan.Invocation
	if e.settings != nil {
		e.settings(&inv)
		for _, flag := range sortedKeys(inv.Set) {
			items = append(items, Override{Name: flagLabel(flag), Source: "settings", Flag: "--" + flag})
		}
	}
	if e.KeepCookies != nil && e.KeepCookies() {
		items = append(items, Override{Name: "Keep cookies", Source: "settings", Flag: "-b and -c (the file's kept jar)"})
	}
	e.mu.Lock()
	if e.mock != "" {
		items = append(items, Override{Name: "base_url", Source: "mock", Flag: "--variable base_url=" + e.mock})
	}
	for _, n := range sortedKeys(e.session) {
		items = append(items, Override{Name: n, Source: "session", Flag: "--variable " + n})
	}
	e.mu.Unlock()
	if p, err := runplan.New(&runplan.Invocation{Cmd: "run", Set: map[string]bool{}}, e.env, e.version); err == nil {
		for _, src := range p.Provenance {
			o := Override{Name: src.Setting, Origin: src.Origin, Flag: src.Setting, Source: "environment"}
			if filepath.IsAbs(src.Origin) {
				o.Source = "config file"
			}
			items = append(items, o)
		}
	}
	if items == nil {
		items = []Override{}
	}
	return Overrides{Count: len(items), Items: items}
}

// Digest identifies the current overrides, values included (a Send is
// refused when they changed since the run it would reuse).
func (e *Envs) Digest() string {
	var inv runplan.Invocation
	e.Extend(&inv)
	data, _ := json.Marshal(inv)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// flagLabel names a settings flag for the chip.
func flagLabel(flag string) string {
	switch flag {
	case "proxy":
		return "Proxy"
	case "connect-timeout":
		return "Connect timeout"
	case "retry":
		return "Retries"
	case "cacert":
		return "CA certificate"
	case "cert":
		return "Client certificate"
	case "key":
		return "Client key"
	case "insecure":
		return "Certificates not verified"
	}
	return flag
}

// Service is the environment bindings.
type Service struct{ e *Envs }

// NewService returns the bindings over e.
func NewService(e *Envs) *Service { return &Service{e: e} }

// List returns the project's environments (no secret values).
func (s *Service) List() (*Project, error) { return s.e.List() }

// SetVariable sets or adds a variable (raw: its JSON value).
func (s *Service) SetVariable(env, name string, raw json.RawMessage) error {
	return s.e.SetVariable(env, name, raw)
}

// RemoveVariable removes a variable from every source of env.
func (s *Service) RemoveVariable(env, name string) error { return s.e.RemoveVariable(env, name) }

// SetSecret sets or adds a secret in env's secrets file.
func (s *Service) SetSecret(env, name, secret string) error { return s.e.SetSecret(env, name, secret) }

// MarkSecret moves a variable to a secrets file.
func (s *Service) MarkSecret(env, name string) error { return s.e.MarkSecret(env, name) }

// SetOverride sets a session override (--variable) for env's runs.
func (s *Service) SetOverride(env, name, text string) error { return s.e.SetOverride(env, name, text) }

// RemoveOverride removes a session override.
func (s *Service) RemoveOverride(name string) { s.e.RemoveOverride(name) }

// Overrides lists the overrides of a run (the chip).
func (s *Service) Overrides() Overrides { return s.e.Overrides() }
