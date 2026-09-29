// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/nhtera/sonde/internal/config"
)

// varSourceKind says where a variable outside the document comes from.
type varSourceKind int

const (
	srcEnvironment   varSourceKind = iota // sonde.yaml environments.<env>.variables
	srcVariablesFile                      // a variables_files entry
	srcSecretsFile                        // a secrets_files entry (value never kept)
	srcProcessEnv                         // HURL_VARIABLE_* / SONDE_VARIABLE_*
	srcProcessSecret                      // HURL_SECRET_* / SONDE_SECRET_* (value never kept)
	srcExtra                              // named by the client (the "extraVariables" setting)
)

// varSource is where a variable is defined. It never holds the value.
type varSource struct {
	kind varSourceKind
	file string // sonde.yaml or the variables/secrets file; "" for a process source
}

// projectVars are the variables a document sees from outside itself.
type projectVars struct {
	project string // sonde.yaml path, "" when none applies
	env     string // active environment, "" when none
	names   map[string]varSource
	err     error // sonde.yaml or variables file problem, shown as a diagnostic
	// noFolder is set when the document is outside every workspace folder:
	// it has no configuration at all (not even its own directory's), so
	// the undefined-variable check is skipped for it entirely rather than
	// flagging every use as undefined.
	noFolder bool
}

// configCache holds the sonde.yaml configuration per project directory.
// Discovery is bounded by the workspace folder containing a document (a
// document outside every folder gets no configuration at all, see
// projectVars.noFolder) and, within that, by the CLI's own discovery rules
// (config.ProjectCache: nearest ancestor, stopping at a VCS root, skipping
// an insecure candidate) -- see findSondeYAML. Every layer is re-validated
// by a cheap file stat before being reused: discovery itself (M6, a
// created or deleted sonde.yaml along the searched chain), the loaded
// project (sonde.yaml's own stat) and its resolved variables (every
// variables_files/secrets_files consulted, present or not). An edit is
// therefore picked up on the next call even when the client never reports
// it through workspace/didChangeWatchedFiles.
//
// The server handles one message at a time (server.go's Run loop), so
// nothing here needs a mutex.
type configCache struct {
	s *Server

	// dirs memoizes, per document directory, the governing sonde.yaml path
	// ("" cached too, for a directory with no project in range).
	dirs map[string]dirEntry
	// projects holds one entry per sonde.yaml path that has been loaded.
	projects map[string]*cachedProject

	// Watch registration bookkeeping for registerWatchers.
	watchGlobs []string
	watchID    string
	watchGen   int
}

func newConfigCache(s *Server) *configCache {
	return &configCache{s: s, dirs: map[string]dirEntry{}, projects: map[string]*cachedProject{}}
}

// dirEntry is a memoized sondeYAMLFor result for one directory, plus what
// to re-stat to notice it going stale (M6).
type dirEntry struct {
	path string // "" when none found
	// warning is discovery's own diagnostic, e.g. a candidate skipped as
	// insecure (config.ProjectCache.TakeWarnings), "" when none.
	warning string
	// stamps is the stat of every candidate sonde.yaml between the
	// directory and its workspace folder boundary (inclusive): a create or
	// delete anywhere in that short chain invalidates this result, not
	// just a change to the one file that was actually found (or the fact
	// that none was).
	stamps map[string]fileStamp
}

func (e dirEntry) valid() bool {
	for path, want := range e.stamps {
		if statStamp(path) != want {
			return false
		}
	}
	return true
}

// cachedProject is one loaded sonde.yaml, plus the variable names resolved
// for every environment looked up so far.
type cachedProject struct {
	stamp fileStamp // sonde.yaml's own stat, for revalidation
	proj  *config.Project
	err   error // config.LoadProject's error, if any

	envs map[string]*envNames
}

// envNames is the resolved variable names for one environment, plus the
// stat of every variables_files/secrets_files consulted to build it.
type envNames struct {
	names  map[string]varSource
	err    error
	stamps map[string]fileStamp // absolute path -> stat, for revalidation
}

// fileStamp is enough to notice a file changed, appeared or disappeared
// without reading it. The zero value (exists false) is itself a valid
// stamp for "absent", so recording it for a not-yet-created file still
// invalidates the entry once that file appears (H1).
type fileStamp struct {
	exists  bool
	size    int64
	modTime int64 // UnixNano; time.Time equality is not reliable
}

func statStamp(path string) fileStamp {
	info, err := os.Stat(path)
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{exists: true, size: info.Size(), modTime: info.ModTime().UnixNano()}
}

// variables returns the variables d sees from its project and process
// environment. A process environment variable or secret ranks above
// sonde.yaml, as in a run, so a name defined by both reports the process
// source.
func (c *configCache) variables(d *document) *projectVars {
	pv := c.projectVariables(d)
	for name := range c.s.opt.Environ.VariableEnvVars() {
		pv.names[name] = varSource{kind: srcProcessEnv}
	}
	for name := range c.s.opt.Environ.SecretEnvVars() {
		pv.names[name] = varSource{kind: srcProcessSecret}
	}
	for _, name := range c.s.extraVariables {
		if _, ok := pv.names[name]; !ok {
			pv.names[name] = varSource{kind: srcExtra}
		}
	}
	return pv
}

// projectVariables returns the variables d sees from its sonde.yaml alone.
func (c *configCache) projectVariables(d *document) *projectVars {
	pv := &projectVars{names: map[string]varSource{}}
	if d.path == "" {
		pv.noFolder = true
		return pv
	}
	dir := filepath.Dir(d.path)
	boundary, hasBoundary := c.s.folderContaining(dir)
	if !hasBoundary {
		pv.noFolder = true
		return pv
	}

	sondePath, ok, warning := c.sondeYAMLFor(dir, boundary)
	if warning != "" {
		// A candidate sonde.yaml was skipped as insecure (group/world-
		// writable or foreign-owned): surface that instead of leaving the
		// user staring at "no sonde.yaml environment selected" with no
		// hint why (kongming checkpoint, phase 9 Should-fix 1).
		pv.err = errors.New(warning)
	}
	if !ok {
		return pv
	}
	pv.project = sondePath

	cp := c.loadProject(sondePath)
	if cp.err != nil {
		pv.err = cp.err
		return pv
	}

	pv.env = c.selectEnv(cp.proj)
	en := c.resolveEnv(cp, pv.env)
	if en.err != nil {
		pv.err = en.err
	}
	for name, src := range en.names {
		pv.names[name] = src
	}
	return pv
}

// selectEnv picks the active environment: the client's explicit setting
// (an explicitly empty one counts as unset, L3), else SONDE_ENV, else the
// project's own default.
func (c *configCache) selectEnv(p *config.Project) string {
	var explicit string
	if c.s.env != nil {
		explicit = *c.s.env
	}
	return config.SelectEnv(explicit, c.s.opt.Environ["SONDE_ENV"], p.Defaults.Env)
}

// sondeYAMLFor returns the sonde.yaml governing dir, bounded by its
// workspace folder boundary (see findSondeYAML), memoized and revalidated
// per M6. warning is discovery's own diagnostic (e.g. a candidate skipped
// as insecure), non-empty regardless of whether a usable sonde.yaml was
// still found further up; it is re-derived on every fresh (cache-miss)
// lookup, so it is never stale and never accumulates across documents.
func (c *configCache) sondeYAMLFor(dir, boundary string) (path string, ok bool, warning string) {
	if e, hit := c.dirs[dir]; hit && e.valid() {
		return e.path, e.path != "", e.warning
	}

	path, _, warning = findSondeYAML(dir, boundary)
	c.dirs[dir] = dirEntry{path: path, warning: warning, stamps: discoveryStamps(dir, boundary)}
	return path, path != "", warning
}

// discoveryStamps stats every candidate sonde.yaml between dir and
// boundary (inclusive): there are only a few, so stat-checking all of them
// on the next lookup is cheap, and it is what lets sondeYAMLFor notice a
// sonde.yaml created or deleted anywhere in that chain (M6).
func discoveryStamps(dir, boundary string) map[string]fileStamp {
	stamps := map[string]fileStamp{}
	cur := dir
	for {
		candidate := filepath.Join(cur, config.ProjectFileName)
		stamps[candidate] = statStamp(candidate)
		if cur == boundary {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return stamps
}

// findSondeYAML returns the sonde.yaml governing dir under the CLI's own
// discovery rules (config.ProjectCache: nearest ancestor, stopping at a
// VCS root, skipping a group/world-writable or foreign-owned candidate --
// see docs/sonde-yaml.md, Discovery), additionally bounded by the
// workspace folder containing dir (M1 in the phase 9 LSP review): a result
// at or above that folder is rejected, and so is one reachable only
// through a symbolic link leaving it, even when the nominal path looks
// like it is inside.
//
// A fresh config.ProjectCache is used for every call: configCache does its
// own revalidation (discoveryStamps), so nothing needs to persist across
// calls here.
//
// A workspace folder narrower than the repository containing it can
// therefore disagree with `sonde run`, which discovers unbounded by any
// folder: a run may use a repo-root sonde.yaml that a narrower folder
// hides from the LSP. Accepted trade-off, not a bug.
//
// warning carries a skipped-candidate diagnostic (config.ProjectCache's
// own "ignored: not owned by the current user, or writable by group or
// others", naming the file) even when a usable sonde.yaml was still found
// further up: the skipped one might be the one the user actually meant.
// The underlying ProjectCache is used once and discarded, so its warnings
// are drained here and never carried over to another call or document.
func findSondeYAML(dir, boundary string) (path string, ok bool, warning string) {
	realBoundary, err := filepath.EvalSymlinks(boundary)
	if err != nil {
		return "", false, ""
	}
	pc := config.NewProjectCache()
	path, ok, err = pc.FindProject(dir)
	warning = strings.Join(pc.TakeWarnings(), "; ")
	if err != nil || !ok {
		return "", false, warning
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false, warning
	}
	realDir := filepath.Dir(realPath)
	if realDir != realBoundary && !strings.HasPrefix(realDir, realBoundary+string(filepath.Separator)) {
		return "", false, warning
	}
	return path, true, warning
}

// folderContaining returns the workspace folder containing dir (the
// longest matching one), or ok=false when dir is inside none of them. "/"
// (or, on Windows, a drive root) is handled specially since it already
// ends in a separator: appending another before the prefix check would
// never match anything (L16).
func (s *Server) folderContaining(dir string) (folder string, ok bool) {
	for _, f := range s.folders {
		if (dir == f || strings.HasPrefix(dir, withTrailingSeparator(f))) && len(f) > len(folder) {
			folder = f
		}
	}
	return folder, folder != ""
}

func withTrailingSeparator(p string) string {
	if strings.HasSuffix(p, string(filepath.Separator)) {
		return p
	}
	return p + string(filepath.Separator)
}

// loadProject returns path's cached project, reloading it when its stat
// (size or modification time, or its absence) no longer matches what was
// last loaded.
func (c *configCache) loadProject(path string) *cachedProject {
	stamp := statStamp(path)
	if cp, hit := c.projects[path]; hit && cp.stamp == stamp {
		return cp
	}

	cp := &cachedProject{stamp: stamp, envs: map[string]*envNames{}}
	switch {
	case !stamp.exists:
		cp.err = fmt.Errorf("%s: no longer exists", path)
	default:
		cp.proj, cp.err = config.LoadProject(path)
	}
	c.projects[path] = cp
	c.s.registerWatchers() // a (re)load may have changed what to watch (L10)
	return cp
}

// resolveEnv returns env's variable names for cp, reusing the cached
// result while every variables_files/secrets_files stat it was built from
// still matches.
func (c *configCache) resolveEnv(cp *cachedProject, env string) *envNames {
	if en, hit := cp.envs[env]; hit && en.stampsValid() {
		return en
	}

	en := &envNames{names: map[string]varSource{}, stamps: map[string]fileStamp{}}
	// H1: stamp every referenced path before reading any of them, whether
	// or not the read succeeds -- an absent file still gets a stamp, so it
	// appearing later invalidates this entry, and stamping before reading
	// means a file edited between the stat and the read can't leave a
	// stamp that looks newer than the names actually collected from it.
	if e, ok := cp.proj.Environments[env]; ok {
		for _, rel := range e.VariablesFiles {
			en.stamp(cp.proj.Dir, rel)
		}
		for _, rel := range e.SecretsFiles {
			en.stamp(cp.proj.Dir, rel)
		}
	}

	names, err := cp.proj.VariableNames(env)
	en.err = err
	for name, vs := range names { // M2: VariableNames returns partial names alongside err
		src := varSource{file: vs.File}
		switch {
		case vs.Secret:
			src.kind = srcSecretsFile
		case vs.File != "":
			src.kind = srcVariablesFile
		default:
			src.kind = srcEnvironment
		}
		en.names[name] = src
	}

	cp.envs[env] = en
	c.s.registerWatchers() // a fresh resolve may have changed what to watch (L10)
	return en
}

func (en *envNames) stamp(dir, rel string) {
	en.stamps[filepath.Join(dir, rel)] = statStamp(filepath.Join(dir, rel))
}

func (en *envNames) stampsValid() bool {
	for abs, want := range en.stamps {
		if statStamp(abs) != want {
			return false
		}
	}
	return true
}

// invalidate drops cached configuration touched by changes (all of it when
// changes is nil or empty, e.g. a workspace folder or configuration
// change). A change to any currently watched basename (every sonde.yaml,
// plus every variables_files/secrets_files basename registerWatchers
// tracks) also drops everything: a sonde.yaml change can alter which
// project governs a directory, and this is a stronger, immediate signal
// than the per-call stat checks in sondeYAMLFor/loadProject/resolveEnv,
// which still catch everything else (H1, M6).
func (c *configCache) invalidate(changes []fileEvent) {
	if len(changes) == 0 {
		c.reset()
		return
	}
	watched := c.watchedBasenames()
	for _, ch := range changes {
		if path := uriToPath(ch.URI); path != "" && watched[filepath.Base(path)] {
			c.reset()
			return
		}
	}
}

func (c *configCache) reset() {
	c.dirs = map[string]dirEntry{}
	c.projects = map[string]*cachedProject{}
}

// registerWatchers asks the client to report changes to configuration
// files: every sonde.yaml, plus every variables_files/secrets_files
// basename referenced by a project loaded so far. It re-registers
// (unregistering the previous id first) whenever that set grows, which
// happens the first time a document's sonde.yaml is discovered and again
// whenever a newly loaded project references files no earlier one did.
// Clients that do not support dynamic registration configure their own
// watcher, so this does nothing for them.
//
// Called only from loadProject/resolveEnv's cache-miss paths (L10), never
// on every diagnostics pass: computing the desired glob set is cheap, but
// there is no reason to do it when nothing could have changed.
func (s *Server) registerWatchers() {
	if !s.dynamicWatch {
		return
	}
	globs := s.config.watchedGlobs()
	if slices.Equal(s.config.watchGlobs, globs) {
		return
	}
	oldID := s.config.watchID
	s.config.watchGen++
	newID := fmt.Sprintf("sonde-watch-%d", s.config.watchGen)
	s.config.watchID = newID
	s.config.watchGlobs = globs

	if oldID != "" {
		s.call("client/unregisterCapability", unregistrationParams{
			Unregisterations: []unregistration{{ID: oldID, Method: "workspace/didChangeWatchedFiles"}},
		})
	}
	watchers := make([]fileSystemWatcher, len(globs))
	for i, g := range globs {
		watchers[i] = fileSystemWatcher{GlobPattern: g}
	}
	s.call("client/registerCapability", registrationParams{Registrations: []registration{{
		ID:              newID,
		Method:          "workspace/didChangeWatchedFiles",
		RegisterOptions: didChangeWatchedFilesRegistrationOptions{Watchers: watchers},
	}}})
}

// watchedGlobs returns the glob patterns registerWatchers should watch:
// every sonde.yaml first, then the basename of every variables_files and
// secrets_files entry of every environment of every project loaded so
// far, deduplicated and sorted for a deterministic registration.
func (c *configCache) watchedGlobs() []string {
	seen := map[string]bool{}
	var rest []string
	for _, cp := range c.projects {
		if cp.proj == nil {
			continue
		}
		for _, e := range cp.proj.Environments {
			for _, rel := range slices.Concat(e.VariablesFiles, e.SecretsFiles) {
				g := "**/" + filepath.Base(rel)
				if !seen[g] {
					seen[g] = true
					rest = append(rest, g)
				}
			}
		}
	}
	sort.Strings(rest)
	return append([]string{"**/" + config.ProjectFileName}, rest...)
}

// watchedBasenames is watchedGlobs' patterns with their "**/" prefix
// stripped, for matching a workspace/didChangeWatchedFiles event's file
// name in invalidate.
func (c *configCache) watchedBasenames() map[string]bool {
	out := map[string]bool{}
	for _, g := range c.watchedGlobs() {
		out[strings.TrimPrefix(g, "**/")] = true
	}
	return out
}
