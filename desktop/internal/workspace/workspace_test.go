// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/emit"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/desktop/internal/sandboxtest"
	"github.com/nhtera/sonde/internal/config"
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
	for _, bad := range []string{".git/config", "a/.vscode/x.json", ".", ".envrc", "a/.env"} {
		if _, err := writable(bad); code(err) != apperr.Denied {
			t.Errorf("writable(%q): %v", bad, err)
		}
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
	if fi, _ := os.Stat(p); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
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

// TestDesktopFolderPrompts checks the folder dialog says why it asks:
// the folder an import writes into is not mistaken for the file to import.
func TestDesktopFolderPrompts(t *testing.T) {
	s, _, dir := open(t)
	var got []FolderPrompt
	d := NewDesktop(s, sandboxtest.Open(t, t.TempDir()), handles.New(), func(p FolderPrompt) (string, error) {
		got = append(got, p)
		return dir, nil
	})
	if p, err := d.OpenFolder(); p == nil || err != nil {
		t.Fatalf("open: %v %v", p, err)
	}
	if p, err := d.OpenFolderToImport(); p == nil || err != nil {
		t.Fatalf("open to import: %v %v", p, err)
	}
	if len(got) != 2 || got[0] != openPrompt || got[1] != importPrompt {
		t.Fatalf("prompts %+v", got)
	}
	// Each platform shows the title or the message, not both: both say it.
	if !strings.Contains(got[1].Title, "import into") || !strings.Contains(got[1].Message, "import into") {
		t.Errorf("the import prompt does not say what the folder is for: %+v", got[1])
	}
}

func TestDesktopRecentAndCopy(t *testing.T) {
	s, _, dir := open(t)
	cfg := sandboxtest.Open(t, t.TempDir())
	h := handles.New()
	d := NewDesktop(s, cfg, h, func(FolderPrompt) (string, error) { return dir, nil })
	if _, err := d.OpenFolder(); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	d.pickFolder = func(FolderPrompt) (string, error) { return other, nil }
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
	// A folder deleted since: said, and removed from the list.
	if err := os.RemoveAll(other); err != nil {
		t.Fatal(err)
	}
	if _, err := d.OpenRecent(recent[0].ID); code(err) != apperr.NotFound || !strings.Contains(err.Error(), "no longer there") {
		t.Errorf("deleted recent: %v", err)
	}
	if got := d.Recent(); len(got) != 1 || got[0].Dir == other {
		t.Errorf("deleted folder still recent: %+v", got)
	}
	d.pickFolder = func(FolderPrompt) (string, error) { return "", nil }
	if p, err := d.OpenFolder(); p != nil || err != nil {
		t.Errorf("canceled dialog: %v %v", p, err)
	}

	ext := filepath.Join(t.TempDir(), "spec.json")
	if err := os.WriteFile(ext, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A picked file inside the project is its project path; one outside
	// only its name and a new handle, to copy it in.
	write(t, s.Root().Dir(), "in/picked.hurl", "GET https://h\n")
	in, _ := h.Put(filepath.Join(s.Root().Dir(), "in", "picked.hurl"), handles.OpenFile)
	if p, err := d.PickedFile(in); err != nil || p.Path != "in/picked.hurl" || p.Handle != "" {
		t.Errorf("picked inside: %+v %v", p, err)
	}
	// A secret or dot file is not sent as a body.
	write(t, s.Root().Dir(), "local.secrets", "k=v\n")
	secret, _ := h.Put(filepath.Join(s.Root().Dir(), "local.secrets"), handles.OpenFile)
	if p, err := d.PickedFile(secret); code(err) != apperr.Denied || p != nil {
		t.Errorf("picked a secrets file: %+v %v", p, err)
	}
	picked, _ := h.Put(ext, handles.OpenFile)
	p, err := d.PickedFile(picked)
	if err != nil || p.Path != "" || p.Name != "spec.json" || p.Handle == "" {
		t.Fatalf("picked outside: %+v %v", p, err)
	}
	if _, err := d.PickedFile(picked); code(err) != apperr.Expired {
		t.Errorf("reused handle: %v", err)
	}
	id := p.Handle
	rel, err := d.CopyIntoProject(id, "api")
	if err != nil || rel != "api/spec.json" {
		t.Fatalf("copy: %q %v", rel, err)
	}
	if _, err := d.CopyIntoProject(id, "api"); code(err) != apperr.Expired {
		t.Errorf("reused handle: %v", err)
	}
}

// TestPickedFileSymlink: a symlink pointing outside the project is reported
// as outside with a handle for CopyIntoProject.
func TestPickedFileSymlink(t *testing.T) {
	s, _, _ := open(t)
	cfg := sandboxtest.Open(t, t.TempDir())
	h := handles.New()
	d := NewDesktop(s, cfg, h, func(FolderPrompt) (string, error) { return "", nil })

	// Create a file outside and a symlink inside pointing to it
	outside := filepath.Join(t.TempDir(), "external.json")
	if err := os.WriteFile(outside, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(s.Root().Dir(), "link.json")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Fatal(err)
	}

	handle, _ := h.Put(symlink, handles.OpenFile)
	p, err := d.PickedFile(handle)
	// Should be treated as outside since it resolves to a path outside project
	if err != nil || p.Path != "" || p.Handle == "" {
		t.Errorf("symlink outside not recognized: %+v %v", p, err)
	}
}

// TestPickedFileWithPathTraversal: a path that climbs out of the project
// with ".." is outside it, even when it starts inside.
func TestPickedFileWithPathTraversal(t *testing.T) {
	s, _, _ := open(t)
	cfg := sandboxtest.Open(t, t.TempDir())
	h := handles.New()
	d := NewDesktop(s, cfg, h, func(FolderPrompt) (string, error) { return "", nil })
	projectDir := s.Root().Dir()
	outside := filepath.Join(filepath.Dir(projectDir), "outside-"+filepath.Base(projectDir)+".txt")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })
	climbing := filepath.Join(projectDir, "api") + string(filepath.Separator) + ".." + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Base(outside)
	handle, _ := h.Put(climbing, handles.OpenFile)
	p, err := d.PickedFile(handle)
	if err != nil || p.Path != "" || p.Handle == "" || p.Name != filepath.Base(outside) {
		t.Errorf("a path climbing out of the project: %+v %v", p, err)
	}
}

// TestPickedFileProjectDirItself: a folder is not a file to send (the
// project's own folder included).
func TestPickedFileProjectDirItself(t *testing.T) {
	s, _, _ := open(t)
	cfg := sandboxtest.Open(t, t.TempDir())
	h := handles.New()
	d := NewDesktop(s, cfg, h, func(FolderPrompt) (string, error) { return "", nil })

	projectDir := s.Root().Dir()
	handle, _ := h.Put(projectDir, handles.OpenFile)
	if p, err := d.PickedFile(handle); code(err) != apperr.Invalid || p != nil {
		t.Errorf("a folder was picked as a file: %+v %v", p, err)
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

// TestSecretsNeverRead: the page cannot read or overwrite secrets files,
// dot files or the files a project lists as secrets.
func TestSecretsNeverRead(t *testing.T) {
	s, _, dir := open(t)
	write(t, dir, "secrets/local.secrets", "password=x\n")
	write(t, dir, "env/prod.env", "token=y\n")
	write(t, dir, ".env", "k=v\n")
	s.Secret = func(rel string) bool { return rel == "env/prod.env" }
	for _, f := range []string{"secrets/local.secrets", "env/prod.env", ".env", ".git/config"} {
		if _, err := s.Read(f); code(err) != apperr.Denied {
			t.Errorf("Read(%q): %v", f, err)
		}
		if _, err := s.Duplicate(f); err == nil {
			t.Errorf("Duplicate(%q) worked", f)
		}
		if _, err := s.Save(f, "x", ""); code(err) != apperr.Denied && code(err) != apperr.Conflict {
			t.Errorf("Save(%q): %v", f, err)
		}
	}
	if _, err := s.Save("secrets/new.secrets", "x", ""); code(err) != apperr.Denied {
		t.Errorf("a new secrets file: %v", err)
	}
	// Renaming would make a secrets file readable (or hide a file).
	write(t, dir, "notes.txt", "x")
	for _, r := range [][2]string{
		{"secrets/local.secrets", "local.txt"},
		{"env/prod.env", "prod.txt"},
		{"secrets", "plain"},
		{"env", "plain"},
		{"notes.txt", "notes.secrets"},
	} {
		if _, err := s.Rename(r[0], r[1]); code(err) != apperr.Denied {
			t.Errorf("Rename(%q, %q): %v", r[0], r[1], err)
		}
	}
	if _, err := s.Rename("notes.txt", "notes.md"); err != nil {
		t.Errorf("a plain rename: %v", err)
	}
	// The tree marks a project secrets file whatever its extension.
	tr, err := s.Tree()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range tr.Children {
		for _, f := range d.Children {
			if f.Path == "env/prod.env" && f.Kind != KindSecrets {
				t.Errorf("env/prod.env is %q in the tree", f.Kind)
			}
		}
	}
}

// TestPollingLargeProject: a project past the watch limit is polled, and
// outside changes are still reported.
func TestPollingLargeProject(t *testing.T) {
	dir := t.TempDir()
	for i := range maxWatched + 5 {
		write(t, dir, fmt.Sprintf("d/f%05d.txt", i), "x")
	}
	rec := &emit.Recorder{}
	s := New(rec)
	if _, err := s.Open(dir); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.watcher == nil || s.watcher.fs != nil {
		t.Fatal("a large project must be polled")
	}
	time.Sleep(50 * time.Millisecond)
	write(t, dir, "d/new.hurl", "GET https://x\n")
	deadline := time.Now().Add(3 * pollEvery)
	for time.Now().Before(deadline) {
		for _, ev := range rec.Events() {
			if c, ok := ev.Data.(Changed); ok && strings.Contains(strings.Join(c.Paths, ","), "d/new.hurl") {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the poller did not report the new file")
}

// TestFilter: a query matches a request's headers and body, not only its
// method and URL; .sonde files are request files too.
func TestFilter(t *testing.T) {
	s, _, dir := open(t)
	write(t, dir, "a.hurl", "# Login\nPOST {{base}}/login\nX-Client: mobile\n{\"user\": \"ada\"}\nHTTP 200\n\nGET {{base}}/me\nHTTP 200\n")
	write(t, dir, "b.sonde", "GET {{base}}/events\nAccept: text/event-stream\nHTTP 200\n")
	write(t, dir, "x.secrets", "mobile=1\n")
	for q, want := range map[string][]Match{
		"MOBILE":       {{File: "a.hurl", Entry: 1, Line: 3, Text: "X-Client: mobile"}},
		"ada":          {{File: "a.hurl", Entry: 1, Line: 4, Text: `{"user": "ada"}`}},
		"/me":          {{File: "a.hurl", Entry: 2, Line: 7, Text: "GET {{base}}/me"}},
		"event-stream": {{File: "b.sonde", Entry: 1, Line: 2, Text: "Accept: text/event-stream"}},
		"  ":           {},
	} {
		got, err := s.Filter(q)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Errorf("Filter(%q) = %+v, want %+v", q, got, want)
		}
	}
}

func TestDuplicateFolder(t *testing.T) {
	s, _, dir := open(t)
	write(t, dir, "api/.cache/x", "x\n")
	write(t, dir, "api/nested/c.hurl", "GET https://c\n")
	got, err := s.Duplicate("api")
	if err != nil || got != "api copy" {
		t.Fatalf("Duplicate(api) = %q, %v", got, err)
	}
	for _, f := range []string{"users.hurl", "data.csv", "nested/c.hurl"} {
		want, _ := os.ReadFile(filepath.Join(dir, "api", filepath.FromSlash(f)))
		if data, err := os.ReadFile(filepath.Join(dir, "api copy", filepath.FromSlash(f))); err != nil || string(data) != string(want) {
			t.Errorf("api copy/%s: %q %v", f, data, err)
		}
	}
	// Dot files stay out, as in the tree.
	if _, err := os.Stat(filepath.Join(dir, "api copy", ".cache")); !os.IsNotExist(err) {
		t.Errorf(".cache copied: %v", err)
	}
	if got, err := s.Duplicate("api"); err != nil || got != "api copy 2" {
		t.Errorf("second copy %q %v", got, err)
	}
	if _, err := s.Duplicate(".git"); err == nil {
		t.Error("a dot folder duplicated")
	}
	// Secrets never get a copy the page could read: neither a *.secrets
	// file nor one sonde.yaml declares.
	write(t, dir, "env/prod.env", "TOKEN=x\n")
	write(t, dir, "env/local.secrets", "password=x\n")
	write(t, dir, "env/notes.hurl", "GET https://n\n")
	s.Secret = func(rel string) bool { return rel == "env/prod.env" }
	got, err = s.Duplicate("env")
	if err != nil || got != "env copy" {
		t.Fatalf("Duplicate(env) = %q, %v", got, err)
	}
	for _, f := range []string{"prod.env", "local.secrets"} {
		if _, err := os.Stat(filepath.Join(dir, "env copy", f)); !os.IsNotExist(err) {
			t.Errorf("%s copied: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "env copy", "notes.hurl")); err != nil {
		t.Errorf("notes.hurl not copied: %v", err)
	}
}

// TestCreateProject: a new project is a folder of its name in the
// location, with a starter sonde.yaml (local), opened; a name taken, or
// not a folder name, is refused; the location is picked with its prompt.
func TestCreateProject(t *testing.T) {
	s, _, _ := open(t)
	var prompts []FolderPrompt
	d := NewDesktop(s, sandboxtest.Open(t, t.TempDir()), handles.New(), func(p FolderPrompt) (string, error) {
		prompts = append(prompts, p)
		return "", nil
	})
	parent := filepath.Join(t.TempDir(), "made", "here")
	p, err := d.CreateProject(parent, "  NMK data  ")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(parent, "NMK data")
	if got, _ := filepath.EvalSymlinks(p.Dir); got != mustEval(t, dir) {
		t.Errorf("opened %s, want %s", p.Dir, dir)
	}
	proj, err := config.LoadProject(filepath.Join(dir, "sonde.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := proj.Environments["local"]; !ok || proj.Defaults.Env != "local" {
		t.Errorf("starter sonde.yaml: %+v", proj)
	}
	if _, err := d.CreateProject(parent, "NMK data"); code(err) != apperr.Conflict {
		t.Errorf("a name taken: %v", err)
	}
	for _, name := range []string{"", " ", "..", ".hidden", "a/b", `a\b`, "c:d", strings.Repeat("x", 101)} {
		if _, err := d.CreateProject(parent, name); code(err) != apperr.Invalid {
			t.Errorf("name %q: %v", name, err)
		}
	}
	if _, err := d.CreateProject("relative", "x"); code(err) != apperr.Invalid {
		t.Errorf("a relative location: %v", err)
	}
	if dir, err := d.PickProjectsDir(); dir != "" || err != nil || len(prompts) != 1 || prompts[0] != locationPrompt {
		t.Errorf("pick: %q %v %+v", dir, err, prompts)
	}
	if d.ProjectsDir() == "" || filepath.Base(d.ProjectsDir()) != "Sonde" {
		t.Errorf("default location %q", d.ProjectsDir())
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	got, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
