// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"

	yaml "go.yaml.in/yaml/v3"

	"github.com/nhtera/sonde/internal/value"
)

// ProjectFileName is the name sonde.yaml discovery looks for.
const ProjectFileName = "sonde.yaml"

// maxProjectFileBytes caps sonde.yaml itself and every file its
// environments reference (variables_files, secrets_files): all are
// untrusted repository content (architecture.md §9), so none of them
// gets an unbounded read.
const maxProjectFileBytes = 1 << 20 // 1 MiB

// Project is a loaded and validated sonde.yaml file: schema owned by
// docs/sonde-yaml.md. It never grants file access beyond its own
// directory (architecture.md §9): variables_files and secrets_files are
// read through a sandbox rooted there.
type Project struct {
	// Path is the sonde.yaml file this project was loaded from.
	Path string
	// Dir is the directory containing Path; variables_files and
	// secrets_files resolve relative to it.
	Dir string
	// Version is the schema version; LoadProject rejects anything but 1.
	Version int
	// Environments is every "environments:" entry, by name.
	Environments map[string]Environment
	// Defaults holds "defaults:".
	Defaults Defaults
	// OpenAPI holds "openapi:" (nil when absent).
	OpenAPI *OpenAPI

	// overlay replaces the content of files the project reads, by path
	// relative to Dir (an edit being validated).
	overlay map[string][]byte
}

// Environment is one "environments:<name>:" entry.
type Environment struct {
	// Variables is the "variables:" map, already typed.
	Variables map[string]value.Value
	// VariablesFiles is "variables_files:", as written in sonde.yaml
	// (relative to Dir), applied in order after Variables.
	VariablesFiles []string
	// SecretsFiles is "secrets_files:", as written in sonde.yaml
	// (relative to Dir).
	SecretsFiles []string
	// Secrets is "secrets:", the names of the secrets the environment
	// needs: their values come from its secrets files (which may then be
	// missing, as on a fresh clone) or from any other secret source
	// (SONDE_SECRET_*, --secret); MissingSecrets says which none sets.
	Secrets []string
}

// Defaults is a sonde.yaml "defaults:" block.
type Defaults struct {
	// Env is the environment used when neither --env nor SONDE_ENV is set.
	Env string
	// Jobs is the default --jobs value; 0 means unset (engine default).
	Jobs int
}

// projectFileYAML is the strict decoding shape of a sonde.yaml document.
// Every field it does not list is an unknown-key error.
type projectFileYAML struct {
	Version      int                        `yaml:"version"`
	Environments map[string]environmentYAML `yaml:"environments"`
	Defaults     defaultsYAML               `yaml:"defaults"`
	OpenAPI      *openAPIYAML               `yaml:"openapi"`
}

type environmentYAML struct {
	Variables      map[string]yaml.Node `yaml:"variables"`
	VariablesFiles []string             `yaml:"variables_files"`
	SecretsFiles   []string             `yaml:"secrets_files"`
	Secrets        []string             `yaml:"secrets"`
}

type defaultsYAML struct {
	Env  string    `yaml:"env"`
	Jobs yaml.Node `yaml:"jobs"`
}

// maxDefaultsJobs bounds "defaults.jobs": an untrusted sonde.yaml must not
// be able to size a run's parallelism (goroutines, HTTP clients, file
// descriptors) arbitrarily.
const maxDefaultsJobs = 64

// jobsFromNode converts a "defaults.jobs" node to its int value: absent
// (a zero Node) means unset (0, the engine's own default); anything but
// an integer in [0, maxDefaultsJobs] is rejected with its line.
func jobsFromNode(node yaml.Node) (int, error) {
	if node.IsZero() {
		return 0, nil
	}
	if node.Kind != yaml.ScalarNode || node.ShortTag() != "!!int" {
		return 0, fmt.Errorf("line %d: defaults.jobs must be a non-negative integer", node.Line)
	}
	n, err := strconv.Atoi(node.Value)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("line %d: defaults.jobs must be a non-negative integer, got %s", node.Line, node.Value)
	}
	if n > maxDefaultsJobs {
		return 0, fmt.Errorf("line %d: defaults.jobs must be at most %d, got %d", node.Line, maxDefaultsJobs, n)
	}
	return n, nil
}

// LoadProject reads and strictly validates the sonde.yaml file at path: an
// unknown key or a value of the wrong type is rejected with the offending
// line; version must be 1; every variables_files/secrets_files path is
// checked against the sandbox rooted at path's directory (an absolute
// path, a path escaping that directory with "..", or one that would
// escape it through a symbolic link is rejected), without requiring the
// referenced file to exist yet (only the environment actually selected by
// Resolve needs to exist).
func LoadProject(path string) (*Project, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", path)
	}
	if info.Size() > maxProjectFileBytes {
		return nil, fmt.Errorf("%s: larger than %d bytes", path, maxProjectFileBytes)
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is caller-provided (discovery or --config)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return loadProjectData(path, data)
}

// loadProjectData is LoadProject on data, the content of the sonde.yaml at
// path.
func loadProjectData(path string, data []byte) (*Project, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var raw projectFileYAML
	if err := dec.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s: empty file", path)
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := rejectExtraDocument(dec); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if raw.Version != 1 {
		return nil, fmt.Errorf("%s: version must be 1, got %d", path, raw.Version)
	}
	jobs, err := jobsFromNode(raw.Defaults.Jobs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	dir := filepath.Dir(path)
	root, err := newProjectSandbox(dir)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	defer root.Close()

	environments := make(map[string]Environment, len(raw.Environments))
	for name, envYAML := range raw.Environments {
		env, err := buildEnvironment(path, root, envYAML)
		if err != nil {
			return nil, err
		}
		environments[name] = env
	}

	openAPI, err := buildOpenAPI(root, raw.OpenAPI)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return &Project{
		Path:         path,
		Dir:          dir,
		Version:      raw.Version,
		Environments: environments,
		Defaults:     Defaults{Env: raw.Defaults.Env, Jobs: jobs},
		OpenAPI:      openAPI,
	}, nil
}

// rejectExtraDocument errors if dec has a second YAML document: sonde.yaml
// is exactly one document, and silently using only the first (YAML's own
// default) would let a second "version: 1" document hide unnoticed below
// the one actually applied.
func rejectExtraDocument(dec *yaml.Decoder) error {
	var extra yaml.Node
	err := dec.Decode(&extra)
	switch {
	case err == nil:
		return fmt.Errorf("line %d: only one YAML document is allowed", extra.Line)
	case errors.Is(err, io.EOF):
		return nil
	default:
		return err
	}
}

// buildEnvironment converts one environments YAML entry, typing its
// variables and checking every file path against root.
func buildEnvironment(path string, root *projectSandbox, envYAML environmentYAML) (Environment, error) {
	var env Environment
	if len(envYAML.Variables) > 0 {
		env.Variables = make(map[string]value.Value, len(envYAML.Variables))
		for name, node := range envYAML.Variables {
			node := node
			v, err := variableFromNode(&node)
			if err != nil {
				return Environment{}, fmt.Errorf("%s: variable %q: %w", path, name, err)
			}
			env.Variables[name] = v
		}
	}
	for _, p := range envYAML.VariablesFiles {
		if err := root.checkPath(p); err != nil {
			return Environment{}, fmt.Errorf("%s: variables_files: %w", path, err)
		}
	}
	env.VariablesFiles = envYAML.VariablesFiles
	for _, p := range envYAML.SecretsFiles {
		if err := root.checkPath(p); err != nil {
			return Environment{}, fmt.Errorf("%s: secrets_files: %w", path, err)
		}
	}
	env.SecretsFiles = envYAML.SecretsFiles
	seen := map[string]bool{}
	for _, name := range envYAML.Secrets {
		switch {
		case !secretNameRE.MatchString(name):
			return Environment{}, fmt.Errorf("%s: secrets: %q is not a variable name", path, name)
		case seen[name]:
			return Environment{}, fmt.Errorf("%s: secrets: %q is listed twice", path, name)
		}
		seen[name] = true
	}
	env.Secrets = envYAML.Secrets
	return env, nil
}

// secretNameRE is a name "secrets:" lists: one a secrets file or
// --secret can define and a {{name}} can use.
var secretNameRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

// ProjectCache finds the nearest ancestor sonde.yaml for a directory,
// memoizing the result per directory so a repeated lookup (common across
// input files sharing a directory tree) walks the filesystem only once.
// It is safe for concurrent use.
type ProjectCache struct {
	mu       sync.Mutex
	found    map[string]projectLookup
	warnings []string
}

type projectLookup struct {
	path string
	ok   bool
	err  error
}

// NewProjectCache returns an empty ProjectCache.
func NewProjectCache() *ProjectCache {
	return &ProjectCache{found: make(map[string]projectLookup)}
}

// TakeWarnings drains and returns every discovery warning accumulated so
// far (for example, a candidate sonde.yaml ignored for failing the
// ownership/permission check — see docs/sonde-yaml.md, Discovery),
// clearing them. A caller that never calls it simply never sees the
// warnings; LoadProject and Resolve never fail because of them. Safe for
// concurrent use.
func (c *ProjectCache) TakeWarnings() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	w := c.warnings
	c.warnings = nil
	return w
}

func (c *ProjectCache) warn(msg string) {
	c.mu.Lock()
	c.warnings = append(c.warnings, msg)
	c.mu.Unlock()
}

// FindProject returns the path of the nearest sonde.yaml above (and
// including) dir: dir itself, then each parent in turn, stopping at the
// first sonde.yaml found, at a directory containing ".git" (inclusive:
// that directory's own sonde.yaml is still eligible, but discovery never
// looks above it), or at the filesystem root. ok is false when none
// exists in that range. A candidate sonde.yaml that is not owned by the
// current user, or that is writable by group or others, is treated as
// absent (a warning is recorded, see TakeWarnings) rather than used: see
// docs/sonde-yaml.md, Discovery, on why the walk is bounded this way.
func (c *ProjectCache) FindProject(dir string) (path string, ok bool, err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false, fmt.Errorf("sonde.yaml discovery: %w", err)
	}
	r := c.find(abs)
	return r.path, r.ok, r.err
}

// find looks up dir, filling the cache for dir and every ancestor it has
// to visit along the way. The lock is held only for map access, never
// across the recursive call or the filesystem stat.
func (c *ProjectCache) find(dir string) projectLookup {
	c.mu.Lock()
	r, hit := c.found[dir]
	c.mu.Unlock()
	if hit {
		return r
	}

	r = c.findUncached(dir)

	c.mu.Lock()
	c.found[dir] = r
	c.mu.Unlock()
	return r
}

func (c *ProjectCache) findUncached(dir string) projectLookup {
	candidate := filepath.Join(dir, ProjectFileName)
	info, err := os.Stat(candidate)
	switch {
	case err == nil && !info.IsDir():
		if !candidateIsSecure(info) {
			c.warn(fmt.Sprintf("%s: ignored (not owned by the current user, or writable by group or others)", candidate))
			return c.ascend(dir)
		}
		return projectLookup{path: candidate, ok: true}
	case err == nil, errors.Is(err, fs.ErrNotExist):
		return c.ascend(dir)
	default:
		return projectLookup{err: err}
	}
}

// ascend continues discovery at dir's parent, unless dir is itself a VCS
// root (it contains ".git"), in which case the walk stops there: nothing
// above the repository being tested should be able to configure a run
// inside it.
func (c *ProjectCache) ascend(dir string) projectLookup {
	if isVCSRoot(dir) {
		return projectLookup{}
	}
	parent := filepath.Dir(dir)
	if parent == dir {
		return projectLookup{}
	}
	return c.find(parent)
}

// isVCSRoot reports whether dir has a ".git" entry (a directory for a
// normal checkout, a file for a worktree or submodule); either marks dir
// as a repository root.
func isVCSRoot(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}
