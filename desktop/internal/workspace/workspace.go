// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package workspace is the open project: its tree, request index, file
// reads and saves, and file operations, with a watcher that reports
// changes made outside the app. Paths from the page are project-relative
// and slash-separated; writes never go into dot folders.
package workspace

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/internal/sandbox"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/syntaxedit"
)

// Events.
const (
	TopicOpened  = "ws:opened"  // data: Project
	TopicChanged = "ws:changed" // data: Changed
)

// Changed lists project paths changed outside the app.
type Changed struct {
	Paths []string `json:"paths"`
}

// Project is the open project.
type Project struct {
	Name string `json:"name"`
	// Dir is the project folder, for display.
	Dir string `json:"dir"`
}

// FileText is a file's text and the hash to pass back to Save.
type FileText struct {
	Path string `json:"path"`
	Text string `json:"text"`
	Hash string `json:"hash"`
}

// Workspace is the open project, for the app's services. The page reaches
// it through Service.
type Workspace struct {
	emit emit.Emitter

	mu      sync.Mutex
	root    *sandbox.Root
	watcher *watcher
	echoes  map[string]string // path -> hash of our own last save

	// Secret reports whether a project path is a secrets file of the
	// project (its sonde.yaml's secrets_files): the page never reads or
	// writes one; envsvc edits secrets. Set by the app.
	Secret func(rel string) bool
	// Opened runs after a project opens (recovering an unfinished
	// edit, resetting session state). Set by the app.
	Opened func()
}

// New returns a workspace with no project open.
func New(e emit.Emitter) *Workspace {
	return &Workspace{emit: e, echoes: map[string]string{}}
}

// Open opens the project in dir (an absolute folder the app chose: a
// dialog, the recent list or --root), closing the previous one.
func (s *Workspace) Open(dir string) (*Project, error) {
	root, err := sandbox.Open(dir)
	if err != nil {
		return nil, err
	}
	if fi, err := root.Stat("."); err != nil || !fi.IsDir() {
		_ = root.Close()
		return nil, apperr.New(apperr.Invalid, "not a folder: "+dir)
	}
	w, err := watch(root.Dir(), s.changed)
	if err != nil {
		w = nil // no live updates; the tree still works
	}
	s.mu.Lock()
	old, oldWatch := s.root, s.watcher
	s.root, s.watcher, s.echoes = root, w, map[string]string{}
	s.mu.Unlock()
	if oldWatch != nil {
		oldWatch.close()
	}
	if old != nil {
		_ = old.Close()
	}
	if s.Opened != nil {
		s.Opened()
	}
	p := &Project{Name: filepath.Base(root.Dir()), Dir: root.Dir()}
	s.emit.Emit(TopicOpened, p)
	return p, nil
}

// Close closes the project.
func (s *Workspace) Close() {
	s.mu.Lock()
	root, w := s.root, s.watcher
	s.root, s.watcher = nil, nil
	s.mu.Unlock()
	if w != nil {
		w.close()
	}
	if root != nil {
		_ = root.Close()
	}
}

// Root is the open project's root, for other services; nil when none.
func (s *Workspace) Root() *sandbox.Root {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.root
}

func (s *Workspace) project() (*sandbox.Root, error) {
	if r := s.Root(); r != nil {
		return r, nil
	}
	return nil, apperr.New(apperr.NotFound, "no project is open")
}

// Project returns the open project, or nil.
func (s *Workspace) Project() *Project {
	r := s.Root()
	if r == nil {
		return nil
	}
	return &Project{Name: filepath.Base(r.Dir()), Dir: r.Dir()}
}

// Tree returns the project tree.
func (s *Workspace) Tree() (*Node, error) {
	root, err := s.project()
	if err != nil {
		return nil, err
	}
	return tree(root)
}

// Index lists every request of every request file, for search.
func (s *Workspace) Index() ([]Request, error) {
	root, err := s.project()
	if err != nil {
		return nil, err
	}
	t, err := tree(root)
	if err != nil {
		return nil, err
	}
	var files []string
	var walk func(n *Node)
	walk = func(n *Node) {
		if n.Kind == KindRequest {
			files = append(files, n.Path)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(t)
	out := []Request{}
	for _, f := range files {
		src, err := root.ReadFile(filepath.FromSlash(f))
		if err != nil {
			continue
		}
		out = append(out, requestsOf(f, src)...)
	}
	return out, nil
}

// Requests lists the requests of file.
func (s *Workspace) Requests(file string) ([]Request, error) {
	t, err := s.Read(file)
	if err != nil {
		return nil, err
	}
	return requestsOf(t.Path, []byte(t.Text)), nil
}

// protected reports whether the page may not read or write p (OS form):
// dot files and folders (.env, .git/config…), *.secrets files and the
// project's secrets files. Their values reach the page only through the
// environment service, which hides them.
func (s *Workspace) protected(p string) bool {
	slash := filepath.ToSlash(p)
	return dotted(p) || strings.EqualFold(path.Ext(slash), ".secrets") || s.Secret != nil && s.Secret(slash)
}

// Read returns file's text and hash.
func (s *Workspace) Read(file string) (*FileText, error) {
	root, err := s.project()
	if err != nil {
		return nil, err
	}
	p, err := clean(file)
	if err != nil {
		return nil, err
	}
	if s.protected(p) {
		return nil, apperr.New(apperr.Denied, file+" holds secrets or settings the app does not show")
	}
	data, err := root.ReadFile(p)
	if err != nil {
		return nil, notFound(err, file)
	}
	return &FileText{Path: filepath.ToSlash(p), Text: string(data), Hash: hashOf(data)}, nil
}

// Save writes text to file if the file is still the one the page read:
// its current hash must be expectedHash ("" for a file that must not
// exist yet). It returns the new hash.
func (s *Workspace) Save(file, text, expectedHash string) (string, error) {
	root, err := s.project()
	if err != nil {
		return "", err
	}
	p, err := writable(file)
	if err != nil {
		return "", err
	}
	if s.protected(p) {
		return "", apperr.New(apperr.Denied, file+" holds secrets: edit them in Environments")
	}
	data := []byte(text)
	s.mu.Lock() // one save at a time: check and write stay together
	defer s.mu.Unlock()
	perm := fs.FileMode(0o644)
	current, err := root.ReadFile(p)
	switch {
	case err == nil:
		if h := hashOf(current); h != expectedHash {
			return "", &apperr.Error{Code: apperr.Conflict, Message: file + " changed on disk", Data: map[string]string{"hash": h}}
		}
		if fi, err := root.Stat(p); err == nil {
			perm = fi.Mode().Perm()
		}
	case errors.Is(err, fs.ErrNotExist):
		if expectedHash != "" {
			return "", &apperr.Error{Code: apperr.Conflict, Message: file + " was deleted on disk", Data: map[string]string{"hash": ""}}
		}
	default:
		return "", err
	}
	h := hashOf(data)
	s.echoes[filepath.ToSlash(p)] = h
	if err := root.WriteFileAtomic(p, data, perm); err != nil {
		delete(s.echoes, filepath.ToSlash(p))
		return "", err
	}
	return h, nil
}

// changed reports paths the watcher saw, less the echoes of our own saves.
func (s *Workspace) changed(paths []string) {
	s.mu.Lock()
	root := s.root
	var out []string
	for _, p := range paths {
		if h, ok := s.echoes[p]; ok && root != nil {
			if data, err := root.ReadFile(filepath.FromSlash(p)); err == nil && hashOf(data) == h {
				delete(s.echoes, p)
				continue
			}
		}
		out = append(out, p)
	}
	s.mu.Unlock()
	if len(out) > 0 {
		s.emit.Emit(TopicChanged, Changed{Paths: out})
	}
}

// NewFile creates an empty request file name in folder dir ("" for the
// project folder) and returns its path. ".hurl" is added when missing.
func (s *Workspace) NewFile(dir, name string) (string, error) {
	if !strings.HasSuffix(strings.ToLower(name), ".hurl") {
		name += ".hurl"
	}
	if err := checkName(name); err != nil {
		return "", err
	}
	rel := path.Join(dir, name)
	if _, err := s.Save(rel, "", ""); err != nil {
		return "", err
	}
	return rel, nil
}

// NewRequest appends a request (method and URL; {{name}} is a variable)
// to file, whose hash must be expectedHash, and returns the new text.
func (s *Workspace) NewRequest(file, method, url, expectedHash string) (*FileText, error) {
	t, err := s.Read(file)
	if err != nil {
		return nil, err
	}
	if t.Hash != expectedHash {
		return nil, &apperr.Error{Code: apperr.Conflict, Message: file + " changed on disk", Data: map[string]string{"hash": t.Hash}}
	}
	res, err := syntaxedit.AddEntry(t.Path, []byte(t.Text), syntax.EntrySpec{Method: method, URL: textOf(url)})
	if err != nil {
		return nil, apperr.Wrap(apperr.Invalid, err)
	}
	h, err := s.Save(file, string(res.Source), expectedHash)
	if err != nil {
		return nil, err
	}
	return &FileText{Path: t.Path, Text: string(res.Source), Hash: h}, nil
}

var placeholder = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_.-]*)\s*\}\}`)

// textOf splits s into literal parts and {{name}} variables.
func textOf(s string) syntax.Text {
	var t syntax.Text
	last := 0
	for _, m := range placeholder.FindAllStringSubmatchIndex(s, -1) {
		if m[0] > last {
			t = append(t, syntax.Lit(s[last:m[0]]))
		}
		t = append(t, syntax.Var(s[m[2]:m[3]]))
		last = m[1]
	}
	if last < len(s) || len(t) == 0 {
		t = append(t, syntax.Lit(s[last:]))
	}
	return t
}

// Duplicate copies file next to itself ("name copy.hurl", "name copy
// 2.hurl"…) and returns the copy's path.
func (s *Workspace) Duplicate(file string) (string, error) {
	t, err := s.Read(file)
	if err != nil {
		return "", err
	}
	dir, base := path.Split(t.Path)
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 1; i < 1000; i++ {
		name := stem + " copy" + ext
		if i > 1 {
			name = stem + " copy " + strconv.Itoa(i) + ext
		}
		rel := path.Join(dir, name)
		if _, err := s.Save(rel, t.Text, ""); err == nil {
			return rel, nil
		} else if !isConflict(err) {
			return "", err
		}
	}
	return "", apperr.New(apperr.Invalid, "no free name for a copy of "+file)
}

// Rename renames file (or folder) to newName in the same folder and
// returns the new path.
func (s *Workspace) Rename(file, newName string) (string, error) {
	root, err := s.project()
	if err != nil {
		return "", err
	}
	from, err := writable(file)
	if err != nil {
		return "", err
	}
	if err := checkName(newName); err != nil {
		return "", err
	}
	to := filepath.Join(filepath.Dir(from), newName)
	if _, err := writable(filepath.ToSlash(to)); err != nil {
		return "", err
	}
	if _, err := root.Lstat(to); err == nil {
		return "", apperr.New(apperr.Conflict, newName+" already exists")
	}
	if err := root.Rename(from, to); err != nil {
		return "", notFound(err, file)
	}
	return filepath.ToSlash(to), nil
}

// CopyPath returns file's absolute path, for the clipboard.
func (s *Workspace) CopyPath(file string) (string, error) {
	root, err := s.project()
	if err != nil {
		return "", err
	}
	p, err := clean(file)
	if err != nil {
		return "", err
	}
	return filepath.Join(root.Dir(), p), nil
}

// abs returns the absolute path of a writable project path, for the
// window-only file operations.
func (s *Workspace) abs(file string) (string, error) {
	root, err := s.project()
	if err != nil {
		return "", err
	}
	p, err := writable(file)
	if err != nil {
		return "", err
	}
	if _, err := root.Lstat(p); err != nil {
		return "", notFound(err, file)
	}
	return filepath.Join(root.Dir(), p), nil
}

// checkName refuses a file name that is empty, a path, or hidden.
func checkName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) ||
		strings.ContainsRune(name, 0) || strings.HasPrefix(name, ".") {
		return apperr.New(apperr.Invalid, "not a file name: "+name)
	}
	return nil
}

func notFound(err error, file string) error {
	if errors.Is(err, fs.ErrNotExist) {
		return apperr.New(apperr.NotFound, file+" does not exist")
	}
	var denied *os.PathError
	if errors.Is(err, sandbox.ErrDenied) || errors.As(err, &denied) && errors.Is(denied.Err, sandbox.ErrDenied) {
		return apperr.New(apperr.Denied, file+" is outside the project")
	}
	return err
}

func isConflict(err error) bool {
	var e *apperr.Error
	return errors.As(err, &e) && e.Code == apperr.Conflict
}
