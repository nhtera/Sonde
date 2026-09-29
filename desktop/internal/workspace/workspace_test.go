// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/internal/sandbox"
)

func write(t *testing.T, dir, rel, text string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func open(t *testing.T) (*Workspace, *emit.Recorder, string) {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "sonde.yaml", "environments: {}\n")
	write(t, dir, "api/users.hurl", "# List users\nGET {{base}}/users\nHTTP 200\n\n# Créer\nPOST {{base}}/users\n{\"n\": \"é\"}\nHTTP 201\n")
	write(t, dir, "api/data.csv", "a\n1\n")
	write(t, dir, "b.hurl", "GET https://example.org\n")
	write(t, dir, ".git/config", "[core]\n")
	write(t, dir, "node_modules/x/y.hurl", "GET https://x\n")
	write(t, dir, ".hidden.hurl", "GET https://x\n")
	rec := &emit.Recorder{}
	s := New(rec)
	if _, err := s.Open(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, rec, dir
}

func code(err error) string {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestPaths(t *testing.T) {
	for _, bad := range []string{"", "/etc/passwd", "../x", "a/../../x", `a\b`, "C:/x", "x\x00"} {
		if _, err := clean(bad); code(err) != apperr.Denied {
			t.Errorf("clean(%q): %v", bad, err)
		}
	}
	for _, bad := range []string{".git/config", "a/.vscode/x.json", "."} {
		if _, err := writable(bad); code(err) != apperr.Denied {
			t.Errorf("writable(%q): %v", bad, err)
		}
	}
	if p, err := writable("api/.env.hurl"); err != nil || p != filepath.FromSlash("api/.env.hurl") {
		t.Errorf("a dot file in a normal folder: %q %v", p, err)
	}
}

func TestTree(t *testing.T) {
	s, _, _ := open(t)
	root, err := s.Tree()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	var walk func(n *Node, depth int)
	walk = func(n *Node, depth int) {
		got = append(got, fmt.Sprintf("%s%s:%s", strings.Repeat(" ", depth), n.Path, n.Kind))
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	walk(root, 0)
	want := []string{":dir", " api:dir", "  api/data.csv:data", "  api/users.hurl:request", " b.hurl:request", " sonde.yaml:config"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("tree:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestIndexAndRequests(t *testing.T) {
	s, _, _ := open(t)
	idx, err := s.Index()
	if err != nil {
		t.Fatal(err)
	}
	if len(idx) != 3 {
		t.Fatalf("index %+v", idx)
	}
	want := Request{File: "api/users.hurl", Entry: 2, Method: "POST", URL: "{{base}}/users", Title: "Créer", Line: 6}
	if idx[1] != want {
		t.Errorf("index[1] = %+v, want %+v", idx[1], want)
	}
	if idx[0].Title != "List users" || idx[0].Line != 2 {
		t.Errorf("index[0] %+v", idx[0])
	}
}

func TestReadSaveConflicts(t *testing.T) {
	s, _, dir := open(t)
	f, err := s.Read("b.hurl")
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.Save("b.hurl", "GET https://example.org/2\n", f.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save("b.hurl", "x", f.Hash); code(err) != apperr.Conflict {
		t.Errorf("stale hash: %v", err)
	}
	write(t, dir, "b.hurl", "changed outside\n")
	if _, err := s.Save("b.hurl", "x", h); code(err) != apperr.Conflict {
		t.Errorf("outside change: %v", err)
	}
	if _, err := s.Save("new.hurl", "x", "somehash"); code(err) != apperr.Conflict {
		t.Errorf("expected a file that does not exist: %v", err)
	}
	if _, err := s.Save("new.hurl", "x", ""); err != nil {
		t.Errorf("new file: %v", err)
	}
	if _, err := s.Save("new.hurl", "y", ""); code(err) != apperr.Conflict {
		t.Errorf("creating over an existing file: %v", err)
	}
	for _, bad := range []string{".git/hooks/pre-commit", "../out.hurl", "/tmp/x.hurl"} {
		if _, err := s.Save(bad, "x", ""); code(err) != apperr.Denied {
			t.Errorf("Save(%q): %v", bad, err)
		}
	}
	if _, err := s.Read("nope.hurl"); code(err) != apperr.NotFound {
		t.Errorf("missing: %v", err)
	}
}

func TestSaveKeepsMode(t *testing.T) {
	s, _, dir := open(t)
	p := filepath.Join(dir, "b.hurl")
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
	f, _ := s.Read("b.hurl")
	if _, err := s.Save("b.hurl", "GET https://x\n", f.Hash); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
}

func TestWatcherSuppressesOwnSaves(t *testing.T) {
	s, rec, dir := open(t)
	f, _ := s.Read("b.hurl")
	if _, err := s.Save("b.hurl", "GET https://example.org/own\n", f.Hash); err != nil {
		t.Fatal(err)
	}
	time.Sleep(4 * debounce)
	for _, ev := range rec.Events() {
		if ev.Topic == TopicChanged {
			t.Fatalf("own save reported: %+v", ev.Data)
		}
	}
	write(t, dir, "api/users.hurl", "GET https://other\n")
	deadline := time.Now().Add(5 * time.Second)
	for {
		for _, ev := range rec.Events() {
			if c, ok := ev.Data.(Changed); ok && ev.Topic == TopicChanged {
				for _, p := range c.Paths {
					if p == "b.hurl" {
						t.Errorf("own save reported with the outside change: %v", c.Paths)
					}
					if p == "api/users.hurl" {
						return
					}
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("outside change not reported: %+v", rec.Events())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestFileOperations(t *testing.T) {
	s, _, _ := open(t)
	rel, err := s.NewFile("api", "orders")
	if err != nil || rel != "api/orders.hurl" {
		t.Fatalf("NewFile: %q %v", rel, err)
	}
	if _, err := s.NewFile("", ".hidden"); code(err) != apperr.Invalid {
		t.Errorf("hidden name: %v", err)
	}
	if _, err := s.NewFile(".git", "x"); code(err) != apperr.Denied {
		t.Errorf("into .git: %v", err)
	}
	f, _ := s.Read(rel)
	nf, err := s.NewRequest(rel, "GET", "{{base}}/orders/{{ id }}", f.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(nf.Text, "GET {{base}}/orders/{{id}}") {
		t.Errorf("new request:\n%s", nf.Text)
	}
	if _, err := s.NewRequest(rel, "GET", "https://x", f.Hash); code(err) != apperr.Conflict {
		t.Errorf("stale NewRequest: %v", err)
	}
	c1, err := s.Duplicate("b.hurl")
	if err != nil || c1 != "b copy.hurl" {
		t.Fatalf("duplicate: %q %v", c1, err)
	}
	if c2, err := s.Duplicate("b.hurl"); err != nil || c2 != "b copy 2.hurl" {
		t.Fatalf("second duplicate: %q %v", c2, err)
	}
	if _, err := s.Rename("b copy.hurl", "b.hurl"); code(err) != apperr.Conflict {
		t.Errorf("rename over a file: %v", err)
	}
	if r, err := s.Rename("b copy.hurl", "c.hurl"); err != nil || r != "c.hurl" {
		t.Errorf("rename: %q %v", r, err)
	}
	if _, err := s.Rename("c.hurl", "../d.hurl"); code(err) != apperr.Invalid {
		t.Errorf("rename out: %v", err)
	}
	if p, err := s.CopyPath("c.hurl"); err != nil || !filepath.IsAbs(p) {
		t.Errorf("copy path %q %v", p, err)
	}
}

func TestDesktopRecentAndCopy(t *testing.T) {
	s, _, dir := open(t)
	cfg, err := sandbox.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := handles.New()
	d := NewDesktop(s, cfg, h, func() (string, error) { return dir, nil })
	if _, err := d.OpenFolder(); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	d.pickFolder = func() (string, error) { return other, nil }
	if _, err := d.OpenFolder(); err != nil {
		t.Fatal(err)
	}
	recent := d.Recent()
	if len(recent) != 2 || recent[0].Dir != s.Root().Dir() {
		t.Fatalf("recent %+v", recent)
	}
	if _, err := d.OpenRecent(recent[1].ID); err != nil || s.Root().Dir() != recent[1].Dir {
		t.Fatalf("open recent: %v", err)
	}
	if got := d.Recent(); got[0].ID != recent[1].ID || len(got) != 2 {
		t.Errorf("reopened project not first: %+v", got)
	}
	if _, err := d.OpenRecent("nope"); code(err) != apperr.NotFound {
		t.Errorf("unknown recent: %v", err)
	}
	d.pickFolder = func() (string, error) { return "", nil }
	if p, err := d.OpenFolder(); p != nil || err != nil {
		t.Errorf("canceled dialog: %v %v", p, err)
	}

	ext := filepath.Join(t.TempDir(), "spec.json")
	if err := os.WriteFile(ext, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	id, _ := h.Put(ext, handles.OpenFile)
	rel, err := d.CopyIntoProject(id, "api")
	if err != nil || rel != "api/spec.json" {
		t.Fatalf("copy: %q %v", rel, err)
	}
	if _, err := d.CopyIntoProject(id, "api"); code(err) != apperr.Expired {
		t.Errorf("reused handle: %v", err)
	}
}

func TestTreeIndexTiming(t *testing.T) {
	dir := t.TempDir()
	for i := range 1000 {
		write(t, dir, fmt.Sprintf("d%02d/f%04d.hurl", i%50, i), "# Get item\nGET {{base}}/items/1\nHTTP 200\n\nPOST {{base}}/items\n{\"a\": 1}\nHTTP 201\n")
	}
	s := New(&emit.Recorder{})
	if _, err := s.Open(dir); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	start := time.Now()
	if _, err := s.Tree(); err != nil {
		t.Fatal(err)
	}
	idx, err := s.Index()
	if err != nil || len(idx) != 2000 {
		t.Fatalf("index: %d %v", len(idx), err)
	}
	t.Logf("1000-file Tree+Index: %v (budget 300ms on an M1)", time.Since(start))
}
