// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package opencollection

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"github.com/nhtera/sonde/internal/convert"
)

// Size limits for a directory collection (docs/decisions/0002-
// opencollection-mapping.md, "Input layouts"): every read goes through
// readCapped, which enforces both before any content is used.
const (
	maxFileSize  = 16 << 20
	maxTotalSize = 256 << 20
	maxFileCount = 10000
	// maxDirCount and maxDirDepth bound the directory tree itself, not
	// just the files in it: without them, a pathologically wide or deep
	// tree (or a directory symlink cycle entirely inside the collection
	// directory, which os.Root does not refuse — only a cycle leaving it)
	// could exhaust memory or the goroutine stack before any file-count or
	// byte limit would ever trigger.
	maxDirCount = 10000
	maxDirDepth = 64
)

// budget is a directory read's remaining file, directory and byte
// allowance.
type budget struct {
	files int
	bytes int64
	dirs  int
}

func newBudget() *budget { return &budget{files: maxFileCount, bytes: maxTotalSize, dirs: maxDirCount} }

func (b *budget) take(path string, size int64) error {
	if b.files <= 0 {
		return fmt.Errorf("opencollection: more than %d files in the collection", maxFileCount)
	}
	b.files--
	if size > maxFileSize {
		return fmt.Errorf("opencollection: %s: larger than %d MiB", path, maxFileSize>>20)
	}
	b.bytes -= size
	if b.bytes < 0 {
		return fmt.Errorf("opencollection: collection larger than %d MiB total", maxTotalSize>>20)
	}
	return nil
}

// takeDir accounts for one more directory (a folder, or environments/)
// visited during the walk.
func (b *budget) takeDir() error {
	if b.dirs <= 0 {
		return fmt.Errorf("opencollection: more than %d directories in the collection", maxDirCount)
	}
	b.dirs--
	return nil
}

// dirLoad accumulates the non-fatal findings of a directory read: a
// malformed individual file, or an escaping symlink (skipEscape), is a
// Skipped entry or a Warning, never an error that aborts the whole import
// (only an oversized/unreadable input, or the tree itself being too big,
// does — via budget).
type dirLoad struct {
	budget   *budget
	warnings []convert.Warning
	skipped  []convert.Skipped
}

func (d *dirLoad) warnf(path string, err error) {
	d.warnings = append(d.warnings, convert.Warning{Kind: convert.WarnUnsupported,
		Message: fmt.Sprintf("opencollection: %s: %v", path, err)})
}

func (d *dirLoad) skip(name, reason string) {
	d.skipped = append(d.skipped, convert.Skipped{Name: name, Reason: reason})
}

// skipEscape reports whether err is os.Root refusing a symlink at name
// that would leave the collection directory, recording it as a warning
// (and telling the caller to skip that one entry) rather than aborting
// the whole import. os.Root has no exported sentinel for this (only an
// internal errPathEscapes, "path escapes from parent" — stable stdlib
// wording since Go added os.Root), so the check matches that text; any
// other error still aborts (a real I/O failure, or this package's own
// size/file/directory budget).
func (d *dirLoad) skipEscape(name string, err error) bool {
	if err == nil || !strings.Contains(err.Error(), "path escapes") {
		return false
	}
	d.warnings = append(d.warnings, convert.Warning{Kind: convert.WarnUnsupported,
		Message: fmt.Sprintf("opencollection: %s: a symlink here would leave the collection directory; skipped", name)})
	return true
}

// readCapped reads a regular file at p through fsys, enforcing the size
// caps in d.budget before the content is used.
func readCapped(fsys fs.FS, p string, b *budget) ([]byte, error) {
	info, err := fs.Stat(fsys, p)
	if err != nil {
		return nil, fmt.Errorf("opencollection: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("opencollection: %s: not a regular file", p)
	}
	if err := b.take(p, info.Size()); err != nil {
		return nil, err
	}
	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, fmt.Errorf("opencollection: %w", err)
	}
	return data, nil
}

func sortedEntries(fsys fs.FS, dir string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("opencollection: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func isYAMLFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml")
}

func isNamed(name string, stems ...string) bool {
	lower := strings.ToLower(name)
	for _, s := range stems {
		if lower == s+".yml" || lower == s+".yaml" {
			return true
		}
	}
	return false
}

func stemName(name string) string {
	return strings.TrimSuffix(name, path.Ext(name))
}

// loadDirectory reads dir's collection: an opencollection.yml/.yaml root
// file (optional), request and folder files, and environments/ (mapping
// doc, "Input layouts" — "Directory").
func loadDirectory(fsys fs.FS, d *dirLoad) (*document, error) {
	entries, err := sortedEntries(fsys, ".")
	if err != nil {
		return nil, err
	}
	doc := &document{}
	var items []item
	var envFiles []environment
	for _, ent := range entries {
		name := ent.Name()
		switch {
		case ent.IsDir() && name == "environments":
			envs, err := loadEnvironments(fsys, name, d)
			if err != nil {
				if d.skipEscape(name, err) {
					continue
				}
				return nil, err
			}
			envFiles = envs
		case ent.IsDir():
			folder, err := loadFolder(fsys, name, name, d, 1)
			if err != nil {
				if d.skipEscape(name, err) {
					continue
				}
				return nil, err
			}
			items = append(items, folder)
		case isNamed(name, "opencollection"):
			data, err := readCapped(fsys, name, d.budget)
			if err != nil {
				if d.skipEscape(name, err) {
					continue
				}
				return nil, err
			}
			var root yaml.Node
			if err := yaml.Unmarshal(data, &root); err != nil {
				d.warnf(name, err)
				continue
			}
			rd, err := decodeDocumentNode(&root)
			if err != nil {
				d.warnf(name, err)
				continue
			}
			doc.OpenCollection = rd.OpenCollection
			doc.Info = rd.Info
			doc.Request = rd.Request
			doc.Docs = rd.Docs
			doc.Config.Environments = append(doc.Config.Environments, rd.Config.Environments...)
			items = append(items, rd.Items...)
			doc.decodeErrors = append(doc.decodeErrors, rd.decodeErrors...)
		case isYAMLFile(name):
			it, err := loadRequestFile(fsys, name, d)
			if err != nil {
				if d.skipEscape(name, err) {
					continue
				}
				return nil, err
			}
			if it != nil {
				items = append(items, *it)
			}
		}
		// Anything else (a dotfile, README, .bru sibling, ...) is ignored.
	}
	doc.Items = items
	doc.Config.Environments = mergeEnvironments(doc.Config.Environments, envFiles, d)
	return doc, nil
}

// loadFolder reads one folder subdirectory: an optional folder.yml/.yaml
// for its own name/defaults/docs, plus its request files and
// subdirectories (mapping doc, "Input layouts"). depth is 1 for a
// top-level folder, bounded by maxDirDepth so a pathologically deep (or
// symlink-cycled) tree can't recurse without limit.
func loadFolder(fsys fs.FS, dir, name string, d *dirLoad, depth int) (item, error) {
	if depth > maxDirDepth {
		return item{}, fmt.Errorf("opencollection: %s: directory nesting exceeds %d levels", dir, maxDirDepth)
	}
	if err := d.budget.takeDir(); err != nil {
		return item{}, err
	}
	entries, err := sortedEntries(fsys, dir)
	if err != nil {
		return item{}, err
	}
	fi := item{Info: itemInfo{Name: name, Type: "folder"}}
	var children []item
	for _, ent := range entries {
		full := path.Join(dir, ent.Name())
		switch {
		case ent.IsDir():
			child, err := loadFolder(fsys, full, ent.Name(), d, depth+1)
			if err != nil {
				if d.skipEscape(full, err) {
					continue
				}
				return item{}, err
			}
			children = append(children, child)
		case isNamed(ent.Name(), "folder"):
			data, err := readCapped(fsys, full, d.budget)
			if err != nil {
				if d.skipEscape(full, err) {
					continue
				}
				return item{}, err
			}
			var meta struct {
				Info    itemInfo    `yaml:"info"`
				Request requestDefs `yaml:"request"`
				Docs    description `yaml:"docs"`
			}
			if err := yaml.Unmarshal(data, &meta); err != nil {
				d.warnf(full, err)
				continue
			}
			if meta.Info.Name != "" {
				fi.Info.Name = meta.Info.Name
			}
			fi.Info.Seq = meta.Info.Seq
			fi.Request, fi.Docs = meta.Request, meta.Docs
		case isYAMLFile(ent.Name()):
			it, err := loadRequestFile(fsys, full, d)
			if err != nil {
				if d.skipEscape(full, err) {
					continue
				}
				return item{}, err
			}
			if it != nil {
				children = append(children, *it)
			}
		}
	}
	fi.Items = children
	return fi, nil
}

// loadRequestFile reads one request file: an http or graphql leaf item.
// A file that fails to parse, or that has neither block, is recorded as
// Skipped and dropped (nil, nil) rather than failing the whole import.
func loadRequestFile(fsys fs.FS, full string, d *dirLoad) (*item, error) {
	data, err := readCapped(fsys, full, d.budget)
	if err != nil {
		return nil, err
	}
	var it item
	if err := yaml.Unmarshal(data, &it); err != nil {
		d.skip(full, "opencollection: "+err.Error())
		return nil, nil
	}
	if it.Info.Name == "" {
		it.Info.Name = stemName(path.Base(full))
	}
	if it.Info.Type == "" {
		switch {
		case it.HTTP != nil:
			it.Info.Type = "http"
		case it.GraphQL != nil:
			it.Info.Type = "graphql"
		default:
			d.skip(full, "opencollection: file has neither an http nor a graphql block")
			return nil, nil
		}
	}
	return &it, nil
}

// loadEnvironments reads every file directly under an environments/
// subdirectory as one Environment each (mapping doc, "Input layouts").
func loadEnvironments(fsys fs.FS, dir string, d *dirLoad) ([]environment, error) {
	entries, err := sortedEntries(fsys, dir)
	if err != nil {
		return nil, err
	}
	var envs []environment
	for _, ent := range entries {
		if ent.IsDir() || !isYAMLFile(ent.Name()) {
			continue
		}
		full := path.Join(dir, ent.Name())
		data, err := readCapped(fsys, full, d.budget)
		if err != nil {
			if d.skipEscape(full, err) {
				continue
			}
			return nil, err
		}
		var env environment
		if err := yaml.Unmarshal(data, &env); err != nil {
			d.warnf(full, err)
			continue
		}
		if env.Name == "" {
			env.Name = stemName(ent.Name())
		}
		envs = append(envs, env)
	}
	return envs, nil
}

// mergeEnvironments folds the environments/ directory's files into the
// root file's inline config.environments, the file winning a name
// collision (mapping doc, "Input layouts").
func mergeEnvironments(base, fromFiles []environment, d *dirLoad) []environment {
	if len(fromFiles) == 0 {
		return base
	}
	out := append([]environment(nil), base...)
	index := make(map[string]int, len(out))
	for i, e := range out {
		index[e.Name] = i
	}
	for _, e := range fromFiles {
		if i, ok := index[e.Name]; ok {
			d.warnings = append(d.warnings, convert.Warning{Kind: convert.WarnUnsupported,
				Message: fmt.Sprintf("opencollection: environment %q defined twice; using the environments/ file", e.Name)})
			out[i] = e
			continue
		}
		index[e.Name] = len(out)
		out = append(out, e)
	}
	return out
}
