// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package convert is the shared writer importers (curl, Postman,
// OpenCollection, .http, OpenAPI) use to turn generated request files into
// files on disk: path sanitizing, collision handling, a sandboxed atomic
// write and a sonde.yaml skeleton that is never overwritten.
package convert

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nhtera/sonde/internal/sandbox"
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

// RawFile is a file other than a request file an importer produced, such
// as a variables file or a secrets stub. Path is relative and
// slash-separated, with its extension; Write sanitizes it like a request
// file's path and keeps it distinct from every other planned file.
type RawFile struct {
	Path string
	Data []byte
	// Keep marks a file a user goes on to fill in, such as a secrets stub:
	// it is written 0o600 and, like sonde.yaml, never overwritten, even
	// with --force.
	Keep bool
}

// Output is everything one importer run produces.
type Output struct {
	// Files are the generated request files.
	Files []GeneratedFile
	// Extra are the other files, written next to the request files.
	Extra []RawFile
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
	// Extra are the relative paths of Output.Extra written (or planned),
	// sorted.
	Extra []string
	// ExtraKept are the Output.Extra files marked Keep that already
	// existed and were left untouched, sorted.
	ExtraKept []string
}

// ErrConflicts is returned by Write when Options.Force is false and one or
// more planned files already exist in DIR: nothing was written.
type ErrConflicts struct{ Files []string }

func (e *ErrConflicts) Error() string {
	return fmt.Sprintf("%d file(s) already exist (use --force to overwrite): %s",
		len(e.Files), strings.Join(e.Files, ", "))
}

// PlannedFile is a file Write writes: its path relative to DIR
// (slash-separated), its content and its permission.
type PlannedFile struct {
	Path string
	Data []byte
	Perm fs.FileMode
}

// Plan computes what Write does with out in dir under opts, without
// writing anything: the Result, and the files to write in order. A
// missing dir has no files yet. Without opts.Force, planned files that
// already exist are an *ErrConflicts.
func Plan(dir string, out Output, opts Options) (*Result, []PlannedFile, error) {
	ext, err := opts.extension()
	if err != nil {
		return nil, nil, err
	}

	used := map[string]bool{ProjectFileName: true}
	planned := planPaths(used, out.Files, ext)
	extra, err := planExtra(used, out.Extra)
	if err != nil {
		return nil, nil, err
	}
	res := &Result{DryRun: opts.DryRun, Files: append([]string(nil), planned...)}
	sort.Strings(res.Files)
	for _, f := range out.Files {
		res.RequestCount += len(f.File.Entries)
	}

	// exists reports whether rel exists in dir (never, when dir does not).
	exists := func(string) (bool, error) { return false, nil }
	if _, err := os.Stat(dir); err != nil && !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("convert: %w", err)
	} else if err == nil {
		root, err := os.OpenRoot(dir)
		if err != nil {
			return nil, nil, fmt.Errorf("convert: %w", err)
		}
		defer func() { _ = root.Close() }()
		exists = func(rel string) (bool, error) {
			_, err := root.Stat(rel)
			if err == nil {
				return true, nil
			}
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, err
		}
	}

	projectExists := false
	if out.ProjectYAML != nil {
		ok, err := exists(ProjectFileName)
		if err != nil {
			return nil, nil, fmt.Errorf("convert: %s: %w", ProjectFileName, err)
		}
		if ok {
			projectExists = true
			res.ProjectSkipped = true
		} else {
			res.Project = ProjectFileName
		}
	}

	var writeExtra []int
	for i, rel := range extra {
		if out.Extra[i].Keep {
			if ok, _ := exists(rel); ok {
				res.ExtraKept = append(res.ExtraKept, rel)
				continue
			}
		}
		writeExtra = append(writeExtra, i)
		res.Extra = append(res.Extra, rel)
	}
	sort.Strings(res.Extra)
	sort.Strings(res.ExtraKept)

	if !opts.Force {
		var conflicts []string
		for _, rel := range planned {
			if ok, _ := exists(rel); ok {
				conflicts = append(conflicts, rel)
			}
		}
		for _, i := range writeExtra {
			if ok, _ := exists(extra[i]); ok {
				conflicts = append(conflicts, extra[i])
			}
		}
		if len(conflicts) > 0 {
			sort.Strings(conflicts)
			return nil, nil, &ErrConflicts{Files: conflicts}
		}
	}

	var files []PlannedFile
	for i, f := range out.Files {
		files = append(files, PlannedFile{Path: planned[i], Data: syntax.Lint(f.File), Perm: 0o644})
	}
	for _, i := range writeExtra {
		perm := fs.FileMode(0o644)
		if out.Extra[i].Keep {
			perm = 0o600
		}
		files = append(files, PlannedFile{Path: extra[i], Data: out.Extra[i].Data, Perm: perm})
	}
	if out.ProjectYAML != nil && !projectExists {
		files = append(files, PlannedFile{Path: ProjectFileName, Data: out.ProjectYAML, Perm: 0o644})
	}
	return res, files, nil
}

// Write formats and writes out into dir under opts. With opts.DryRun set,
// it computes the same plan and returns it without touching the
// filesystem. dir is created if missing; every other write is confined
// inside it (including through a symbolic link) using os.Root.
func Write(dir string, out Output, opts Options) (*Result, error) {
	if !opts.DryRun {
		if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: output directories are 0o755, docs/guides/import-export.md
			return nil, fmt.Errorf("convert: %w", err)
		}
	}
	res, files, err := Plan(dir, out, opts)
	if err != nil || opts.DryRun {
		return res, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("convert: %w", err)
	}
	defer func() { _ = root.Close() }()
	for _, f := range files {
		if err := sandbox.WriteFileAtomicIn(root, f.Path, f.Data, f.Perm); err != nil {
			return nil, fmt.Errorf("convert: %w", err)
		}
	}
	return res, nil
}

// planPaths sanitizes and de-duplicates the relative output path of every
// generated file, in order, appending "."+ext to each.
func planPaths(used map[string]bool, files []GeneratedFile, ext string) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = uniquePath(used, sanitizePath(f.Path)+"."+ext)
	}
	return out
}

// planExtra checks the path of every extra file and marks it used. A path
// must already be in the form Write would give it (StubPath builds one), so
// that files referring to it, such as sonde.yaml, name the file written.
func planExtra(used map[string]bool, files []RawFile) ([]string, error) {
	out := make([]string, len(files))
	for i, f := range files {
		ext := path.Ext(f.Path)
		stem := strings.TrimSuffix(f.Path, ext)
		canonical := sanitizePath(stem) + "." + sanitizeSegment(strings.TrimPrefix(ext, "."))
		if ext == "" || canonical != f.Path {
			return nil, fmt.Errorf("convert: extra file path %q is not canonical (want %q)", f.Path, canonical)
		}
		if used[f.Path] {
			return nil, fmt.Errorf("convert: extra file %q planned twice", f.Path)
		}
		used[f.Path] = true
		out[i] = f.Path
	}
	return out, nil
}

// StubPath returns the path of the secrets stub of environment env,
// "secrets/<env>.secrets" with env sanitized like a path segment, distinct
// from every path already in used, which it records.
func StubPath(env string, used map[string]bool) string {
	return uniquePath(used, "secrets/"+sanitizeSegment(env)+".secrets")
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

// FilePath joins the names of the folders and the request to a
// GeneratedFile Path: a slash in a name is part of the name ("Enable /
// disable"), never a folder of its own.
func FilePath(names []string) string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = separators.Replace(n)
	}
	return strings.Join(out, "/")
}

var separators = strings.NewReplacer("/", " ", "\\", " ")

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

// maxSegment is the longest sanitized segment, in bytes: file systems
// limit a name to 255 bytes, and a segment gets an extension and, while
// written, a temporary suffix.
const maxSegment = 100

// sanitizeSegment converts one path segment to a safe, kebab-case name:
// lowercased, every run of characters other than letters and digits
// becomes one '-', leading/trailing '-' are trimmed, "" becomes "request"
// and a Windows-reserved device name gets a "-file" suffix. A segment
// longer than maxSegment bytes is cut and ends with a short hash of the
// whole.
func sanitizeSegment(s string) string {
	var b strings.Builder
	dash := true // suppress a leading '-'
	for _, r := range s {
		r = unicode.ToLower(r)
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
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
	if len(out) > maxSegment {
		sum := sha256.Sum256([]byte(out))
		cut := maxSegment - 9
		for !utf8.RuneStart(out[cut]) {
			cut--
		}
		out = strings.TrimRight(out[:cut], "-") + "-" + hex.EncodeToString(sum[:4])
	}
	if reservedNames[out] {
		out += "-file"
	}
	return out
}
