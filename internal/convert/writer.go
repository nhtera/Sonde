// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package convert is the shared writer importers (curl, Postman,
// OpenCollection, .http, OpenAPI) use to turn generated request files into
// files on disk: path sanitizing, collision handling, a sandboxed atomic
// write and a sonde.yaml skeleton that is never overwritten.
package convert

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/nhtera/sonde/internal/syntax"
)

// ProjectFileName is the sonde.yaml skeleton's file name inside DIR.
const ProjectFileName = "sonde.yaml"

// GeneratedFile is one request file an importer produced. Path is a
// relative, slash-separated path without extension (e.g. "pets/list-pets");
// Write appends ".hurl" or ".sonde", sanitizes every segment and resolves
// collisions among the files of one Output.
type GeneratedFile struct {
	Path string
	File *syntax.File
}

// Output is everything one importer run produces.
type Output struct {
	// Files are the generated request files.
	Files []GeneratedFile
	// ProjectYAML is an optional sonde.yaml skeleton (config.EmitProject's
	// output); nil means the importer has nothing to add to a project file.
	ProjectYAML []byte
	// Warnings are non-fatal notes about the conversion.
	Warnings []Warning
	// Skipped are input items the importer chose not to convert.
	Skipped []Skipped
}

// Result summarizes a Write (or, with Options.DryRun, the plan Write would
// have carried out).
type Result struct {
	// DryRun is Options.DryRun: Files and Project below were computed but
	// nothing was written.
	DryRun bool
	// Files are the relative paths written (or planned), sorted.
	Files []string
	// RequestCount is the number of entries across every written file.
	RequestCount int
	// Project is the sonde.yaml path written (or planned), "" if Output had
	// no ProjectYAML or DIR/sonde.yaml already existed.
	Project string
	// ProjectSkipped is true when Output.ProjectYAML was set but
	// DIR/sonde.yaml already existed, so it was left untouched.
	ProjectSkipped bool
}

// ErrConflicts is returned by Write when Options.Force is false and one or
// more planned files already exist in DIR: nothing was written.
type ErrConflicts struct{ Files []string }

func (e *ErrConflicts) Error() string {
	return fmt.Sprintf("%d file(s) already exist (use --force to overwrite): %s",
		len(e.Files), strings.Join(e.Files, ", "))
}

// Write formats and writes out into dir under opts. With opts.DryRun set,
// it computes the same plan and returns it without touching the
// filesystem. dir is created if missing; every other write is confined
// inside it (including through a symbolic link) using os.Root.
func Write(dir string, out Output, opts Options) (*Result, error) {
	ext, err := opts.extension()
	if err != nil {
		return nil, err
	}

	planned := planPaths(out.Files, ext)
	res := &Result{DryRun: opts.DryRun, Files: append([]string(nil), planned...)}
	sort.Strings(res.Files)
	for _, f := range out.Files {
		res.RequestCount += len(f.File.Entries)
	}

	if opts.DryRun {
		// A dry run creates nothing: a missing directory has no conflicts.
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if out.ProjectYAML != nil {
				res.Project = ProjectFileName
			}
			return res, nil
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: output directories are 0o755, docs/guides/import-export.md
		return nil, fmt.Errorf("convert: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("convert: %w", err)
	}
	defer func() { _ = root.Close() }()

	projectExists := false
	if out.ProjectYAML != nil {
		if _, err := root.Stat(ProjectFileName); err == nil {
			projectExists = true
			res.ProjectSkipped = true
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("convert: %s: %w", ProjectFileName, err)
		} else {
			res.Project = ProjectFileName
		}
	}

	if !opts.Force {
		var conflicts []string
		for _, rel := range planned {
			if _, err := root.Stat(rel); err == nil {
				conflicts = append(conflicts, rel)
			}
		}
		if len(conflicts) > 0 {
			sort.Strings(conflicts)
			return nil, &ErrConflicts{Files: conflicts}
		}
	}

	if opts.DryRun {
		return res, nil
	}

	for i, f := range out.Files {
		data := syntax.Format(f.File)
		if err := writeFileAtomic(root, planned[i], data, 0o644); err != nil {
			return nil, fmt.Errorf("convert: %w", err)
		}
	}
	if out.ProjectYAML != nil && !projectExists {
		if err := writeFileAtomic(root, ProjectFileName, out.ProjectYAML, 0o644); err != nil {
			return nil, fmt.Errorf("convert: %w", err)
		}
	}
	return res, nil
}

// planPaths sanitizes and de-duplicates the relative output path of every
// generated file, in order, appending "."+ext to each.
func planPaths(files []GeneratedFile, ext string) []string {
	used := make(map[string]bool, len(files))
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = uniquePath(used, sanitizePath(f.Path)+"."+ext)
	}
	return out
}

// uniquePath returns base, or base with a "-2", "-3", ... suffix before its
// extension if base is already in used; the result is added to used.
func uniquePath(used map[string]bool, base string) string {
	if !used[base] {
		used[base] = true
		return base
	}
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d%s", stem, n, ext)
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}

// reservedNames are Windows device names unsafe as a file stem on any
// platform.
var reservedNames = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
	"com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
	"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// sanitizePath splits p on '/' and sanitizes every segment, dropping empty
// ones (from a leading '/', "//" or a trailing '/') and "." and ".."
// segments outright, so the result can never escape upward or reference the
// current directory explicitly. An entirely empty result becomes "request".
func sanitizePath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	var segs []string
	for _, s := range strings.Split(p, "/") {
		if s == "" || s == "." || s == ".." {
			continue
		}
		segs = append(segs, sanitizeSegment(s))
	}
	if len(segs) == 0 {
		return "request"
	}
	return path.Join(segs...)
}

// sanitizeSegment converts one path segment to a safe, kebab-case ASCII
// name: lowercased, every run of characters outside [a-z0-9] becomes one
// '-', leading/trailing '-' are trimmed, "" becomes "request" and a
// Windows-reserved device name gets a "-file" suffix.
func sanitizeSegment(s string) string {
	var b strings.Builder
	dash := true // suppress a leading '-'
	for _, r := range s {
		r = unicode.ToLower(r)
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if out == "" {
		out = "request"
	}
	if reservedNames[out] {
		out += "-file"
	}
	return out
}

// tempName returns a random hidden file name for an atomic write's
// temporary file, in the same directory as base (dir is "." for the root).
func tempName(dir, base string) (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	name := "." + base + ".tmp." + hex.EncodeToString(buf)
	if dir == "." {
		return name, nil
	}
	return path.Join(dir, name), nil
}

// writeFileAtomic writes data to rel inside root via a temporary file in
// the same directory, renamed into place, so a reader never sees a partial
// file; parent directories are created as needed (0o755).
func writeFileAtomic(root *os.Root, rel string, data []byte, perm fs.FileMode) (err error) {
	dir, base := path.Dir(rel), path.Base(rel)
	if dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: output directories are 0o755, docs/guides/import-export.md
			return fmt.Errorf("%s: %w", rel, err)
		}
	}
	tmp, err := tempName(dir, base)
	if err != nil {
		return fmt.Errorf("%s: %w", rel, err)
	}
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return fmt.Errorf("%s: %w", rel, err)
	}
	defer func() {
		if err != nil {
			_ = root.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("%s: %w", rel, err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("%s: %w", rel, err)
	}
	if err = root.Rename(tmp, rel); err != nil {
		return fmt.Errorf("%s: %w", rel, err)
	}
	return nil
}
