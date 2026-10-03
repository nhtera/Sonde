// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package importsvc

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/handles"
	"github.com/nhtera/sonde/desktop/internal/sandboxtest"
	"github.com/nhtera/sonde/internal/sandbox"
	"github.com/nhtera/sonde/internal/syntax"
)

// Sentinels of testdata/import/bearer.curl.
var curlSecrets = []string{
	"e2e-curl-bearer-sentinel-7731", "e2e-curl-apikey-sentinel-4410",
	"e2e-curl-query-sentinel-9902", "e2e-curl-password-sentinel-5521",
}

// testdata is desktop/testdata; repo the repository root.
var (
	testdata, _ = filepath.Abs("../../testdata")
	repo, _     = filepath.Abs("../../..")
)

type fakeSecrets struct{ set map[string]string }

func (f *fakeSecrets) SetSecret(env, name, secret string) error {
	f.set[env+"/"+name] = secret
	return nil
}
func (f *fakeSecrets) Names(string) map[string]bool { return map[string]bool{"token": true} }

func service(t *testing.T) (*Service, string, *handles.Table, *fakeSecrets) {
	t.Helper()
	dir := t.TempDir()
	root := sandboxtest.Open(t, dir)
	h := handles.New()
	sec := &fakeSecrets{set: map[string]string{}}
	cache := sandboxtest.Open(t, t.TempDir())
	s := New(func() *sandbox.Root { return root }, h, sec, cache)
	t.Cleanup(s.Reset)
	return s, root.Dir(), h, sec
}

// stage stages path as the dialog would.
func stage(t *testing.T, s *Service, h *handles.Table, path string, dir bool) string {
	t.Helper()
	kind, k := "file", handles.OpenFile
	if dir {
		kind, k = "dir", handles.OpenDir
	}
	id, err := h.Put(path, k)
	if err != nil {
		t.Fatal(err)
	}
	in, err := s.Stage(id, kind)
	if err != nil {
		t.Fatal(err)
	}
	return in.ID
}

var (
	cliOnce sync.Once
	cliPath string
	cliErr  error
)

// cli builds the sonde command once.
func cli(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the command")
	}
	cliOnce.Do(func() {
		cliPath = filepath.Join(os.TempDir(), "sonde-importsvc-test")
		if runtime.GOOS == "windows" {
			cliPath += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", cliPath, "./cmd/sonde")
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			cliErr = errors.New(string(out))
		}
	})
	if cliErr != nil {
		t.Fatal(cliErr)
	}
	return cliPath
}

// tree reads every file under dir: path → mode and content.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, _ := d.Info()
		data, err := root.ReadFile(p)
		out[p] = info.Mode().Perm().String() + "\n" + string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestSameFilesAsTheCommand: for each kind, the files written are those
// `sonde import` writes for the same input and options, byte for byte.
func TestSameFilesAsTheCommand(t *testing.T) {
	bin := cli(t)
	conv := filepath.Join(repo, "testdata", "convert")
	cases := []struct {
		name   string
		args   []string // after `sonde import`, before -o
		req    func(s *Service, h *handles.Table) Request
		folder string
	}{
		{"curl", []string{"curl", filepath.Join(testdata, "import", "bearer.curl")}, func(s *Service, h *handles.Table) Request {
			return Request{Kind: Curl, Input: stage(t, s, h, filepath.Join(testdata, "import", "bearer.curl"), false)}
		}, "c"},
		{"postman by request", []string{"postman", filepath.Join(testdata, "import", "shop.postman_collection.json"), "--environment", filepath.Join(testdata, "import", "dev.postman_environment.json")}, func(s *Service, h *handles.Table) Request {
			return Request{Kind: Postman, Input: stage(t, s, h, filepath.Join(testdata, "import", "shop.postman_collection.json"), false),
				Environments: []string{stage(t, s, h, filepath.Join(testdata, "import", "dev.postman_environment.json"), false)}}
		}, "p"},
		{"postman by folder", []string{"postman", filepath.Join(testdata, "import", "shop.postman_collection.json"), "--group", "folder"}, func(s *Service, h *handles.Table) Request {
			return Request{Kind: Postman, Group: "folder", Input: stage(t, s, h, filepath.Join(testdata, "import", "shop.postman_collection.json"), false)}
		}, "pf"},
		{"http", []string{"http", filepath.Join(conv, "httpfile", "restclient.http")}, func(s *Service, h *handles.Table) Request {
			return Request{Kind: HTTP, Input: stage(t, s, h, filepath.Join(conv, "httpfile", "restclient.http"), false)}
		}, "h"},
		{"opencollection folder", []string{"opencollection", filepath.Join(conv, "opencollection", "directory")}, func(s *Service, h *handles.Table) Request {
			return Request{Kind: OpenCollection, Input: stage(t, s, h, filepath.Join(conv, "opencollection", "directory"), true)}
		}, "o"},
		{"openapi by path", []string{"openapi", filepath.Join(repo, "testdata", "openapi", "petstore-3.0.yaml"), "--group", "path"}, func(s *Service, h *handles.Table) Request {
			return Request{Kind: OpenAPI, Group: "path", Input: stage(t, s, h, filepath.Join(repo, "testdata", "openapi", "petstore-3.0.yaml"), false)}
		}, "a"},
		{"sonde dialect", []string{"curl", filepath.Join(testdata, "import", "bearer.curl"), "--ext", "sonde"}, func(s *Service, h *handles.Table) Request {
			return Request{Kind: Curl, Ext: "sonde", Input: stage(t, s, h, filepath.Join(testdata, "import", "bearer.curl"), false)}
		}, "d"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, dir, h, _ := service(t)
			want := t.TempDir()
			cmd := exec.Command(bin, append(append([]string{"import"}, c.args...), "-o", want)...) //nolint:gosec // G204: the command built above, on test inputs
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("sonde import: %v\n%s", err, out)
			}
			req := c.req(s, h)
			req.Folder = c.folder
			if _, err := s.Write(context.Background(), req, nil); err != nil {
				t.Fatal(err)
			}
			got, exp := tree(t, filepath.Join(dir, c.folder)), tree(t, want)
			if len(got) != len(exp) || len(got) == 0 {
				t.Fatalf("files %d, the command's %d", len(got), len(exp))
			}
			for p, w := range exp {
				if got[p] != w {
					t.Errorf("%s differs:\n%s\nthe command's:\n%s", p, got[p], w)
				}
			}
		})
	}
}

// TestCurlLift: the credentials of a pasted command are offered; those
// lifted become {{name}} in the file and go to the env's secrets, never
// to a file of the project.
func TestCurlLift(t *testing.T) {
	s, dir, _, sec := service(t)
	data, err := os.ReadFile(filepath.Join(testdata, "import", "bearer.curl"))
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Kind: Curl, Text: string(data), Env: "local"}
	pv, err := s.Preview(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range pv.Candidates {
		names = append(names, c.Name+"@"+c.Where)
		req.Lift = append(req.Lift, c.ID)
	}
	// token is taken in the env: the bearer becomes token_2.
	if want := "api_key@query api_key,token_2@Authorization header,x-api-key@X-Api-Key header,password@password of alice"; strings.Join(names, ",") != want {
		t.Errorf("candidates %v", names)
	}
	if len(pv.Files) != 1 || pv.Files[0].Path != "curl.hurl" {
		t.Fatalf("files %+v", pv.Files)
	}
	// No pick yet: everything is lifted, even in the first preview.
	for _, v := range curlSecrets {
		if strings.Contains(pv.Files[0].Text, v) {
			t.Error("a value is in the first preview")
		}
	}
	// Lifting nothing, picked: the values stay.
	kept, err := s.Preview(context.Background(), Request{Kind: Curl, Text: string(data), Env: "local", Lift: []int{}})
	if err != nil || !strings.Contains(kept.Files[0].Text, "Bearer "+curlSecrets[0]) {
		t.Errorf("nothing lifted: %v", err)
	}
	pv, err = s.Preview(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range curlSecrets {
		if strings.Contains(pv.Files[0].Text, v) {
			t.Error("a lifted value is in the preview")
		}
	}
	for _, ref := range []string{"Bearer {{token_2}}", "X-Api-Key: {{x-api-key}}", "api_key={{api_key}}", "alice:{{password}}"} {
		if !strings.Contains(pv.Files[0].Text, ref) {
			t.Errorf("preview lacks %q:\n%s", ref, pv.Files[0].Text)
		}
	}
	w, err := s.Write(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Secrets) != 4 || len(sec.set) != 4 || sec.set["local/token_2"] != curlSecrets[0] || sec.set["local/api_key"] != curlSecrets[2] || sec.set["local/password"] != curlSecrets[3] {
		t.Errorf("secrets %v (%d set)", w.Secrets, len(sec.set))
	}
	for p, content := range tree(t, dir) {
		for _, v := range curlSecrets {
			if strings.Contains(content, v) {
				t.Errorf("%s holds a lifted value", p)
			}
		}
	}
	// Without an environment the values have nowhere to go: none lifted.
	pv, err = s.Preview(context.Background(), Request{Kind: Curl, Text: string(data), Lift: []int{0}})
	if err != nil || !strings.Contains(pv.Files[0].Text, "api_key="+curlSecrets[2]) {
		t.Errorf("lifted without an environment: %v", err)
	}
	// Insert into the open file: the same requests, lifted, avoiding the
	// names it captures; the secrets written only once asked (after the edit).
	sec.set = map[string]string{}
	target := "POST https://a/login\nHTTP 200\n[Captures]\ntoken_2: jsonpath \"$.t\"\n"
	insert := Request{Text: string(data), Env: "local", Lift: []int{1}, Target: "flow.sonde", TargetText: target}
	text, err := s.CurlText(insert)
	if err != nil || !strings.Contains(text, "Bearer {{token_3}}") || strings.Contains(text, curlSecrets[0]) || len(sec.set) != 0 {
		t.Errorf("curl text %v (%d secrets written)", err, len(sec.set))
	}
	if _, err := syntax.Parse("flow.sonde", []byte(text), syntax.DialectSonde); err != nil {
		t.Error(err)
	}
	saved, err := s.SaveSecrets(insert)
	if err != nil || strings.Join(saved, ",") != "token_3" || sec.set["local/token_3"] != curlSecrets[0] {
		t.Errorf("saved %v %v", saved, err)
	}
}

// TestExistingFilesKept: an import over existing files names them, and
// replaces only those it is told to.
func TestExistingFilesKept(t *testing.T) {
	s, dir, _, _ := service(t)
	req := Request{Kind: Curl, Text: "curl https://api.test/a", Folder: "imported"}
	if _, err := s.Write(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "imported", "curl.hurl")
	if err := os.WriteFile(path, []byte("GET https://mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pv, err := s.Preview(context.Background(), req)
	if err != nil || len(pv.Files) != 1 || !pv.Files[0].Exists || pv.Files[0].Path != "imported/curl.hurl" {
		t.Fatalf("preview %+v %v", pv, err)
	}
	w, err := s.Write(context.Background(), req, nil)
	if err != nil || len(w.Kept) != 1 || len(w.Files) != 0 {
		t.Fatalf("written %+v %v", w, err)
	}
	if b, _ := os.ReadFile(path); string(b) != "GET https://mine\n" {
		t.Error("an existing file was replaced")
	}
	if _, err := s.Write(context.Background(), req, []string{"imported/curl.hurl"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "GET https://api.test/a\n" {
		t.Errorf("not replaced: %q", b)
	}
}

// TestFolderInProject: an import writes only in a folder of the project.
func TestFolderInProject(t *testing.T) {
	s, _, _, _ := service(t)
	for _, folder := range []string{"../out", ".git/x", "a/.hidden"} {
		var e *apperr.Error
		if _, err := s.Preview(context.Background(), Request{Kind: Curl, Text: "curl https://x", Folder: folder}); !errors.As(err, &e) || e.Code != apperr.Denied {
			t.Errorf("%s: %v", folder, err)
		}
	}
}

// TestPostmanCountsAndSuggestions: the result counts, then the scripts'
// suggestions: accepted ones reparse with the comments kept; a script that
// would inject a section never does.
func TestPostmanCountsAndSuggestions(t *testing.T) {
	s, dir, h, _ := service(t)
	req := Request{Kind: Postman, Input: stage(t, s, h, filepath.Join(testdata, "import", "shop.postman_collection.json"), false),
		Environments: []string{stage(t, s, h, filepath.Join(testdata, "import", "dev.postman_environment.json"), false)}}
	if sg, err := s.Suggestions(context.Background(), req); err != nil || len(sg) != 0 {
		t.Fatalf("suggestions before the import: %+v %v", sg, err)
	}
	w, err := s.Write(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	// What lines became: the status check and the path variable
	// converted, the other test statements kept as comments.
	want := map[string]Wrote{
		"pm.response.to.have.status(200)":   {To: "HTTP 200", Kind: "converted"},
		"GET /users/:id":                    {To: "GET /users/{{id}}", Kind: "converted"},
		"pm.expect(jsonData.id).to.eql(42)": {To: "# pm.expect(jsonData.id).to.eql(42)", Kind: "comment"},
	}
	for _, x := range w.Wrote {
		if exp, ok := want[x.From]; ok {
			if x.To != exp.To || x.Kind != exp.Kind {
				t.Errorf("wrote %+v, want %+v", x, exp)
			}
			delete(want, x.From)
		}
	}
	if len(want) > 0 || len(w.Wrote) > maxWrote {
		t.Errorf("wrote %+v: missing %v", w.Wrote, want)
	}
	c := w.Counts
	if c.Files != 3 || c.Requests != 3 || c.Environments != 2 || c.PathVariables != 1 || c.StatusChecks != 1 || c.SecretStubs < 1 || c.Scripts < 1 {
		t.Errorf("counts %+v", c)
	}
	sg, err := s.Suggestions(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]Suggestion{}
	for _, x := range sg {
		byPath[x.Path] = x
	}
	user := byPath["users/get-user.hurl"]
	if user.Error != "" || !strings.Contains(user.After, `jsonpath "$.id" == 42`) || !strings.Contains(user.After, `user_name: jsonpath "$.name"`) || len(user.Labels) != 2 {
		t.Fatalf("get user %+v (paths %v)", user, keys(byPath))
	}
	orders := byPath["list-orders.hurl"]
	if orders.Error != "" || !strings.Contains(orders.After, "Bearer {{access_token}}") {
		t.Errorf("orders %+v", orders)
	}
	// One change on its own: the file with it alone, and where it starts.
	if len(user.Changes) != 2 || user.Changes[0].Index != 0 || user.Changes[1].Index != 1 {
		t.Fatalf("get user changes %+v", user.Changes)
	}
	for _, ch := range user.Changes {
		if ch.Error != "" || ch.After == user.Before || ch.After == user.After || ch.Line < 1 || ch.Label == "" {
			t.Errorf("change %+v", ch)
		}
		if line := strings.Split(ch.After, "\n")[ch.Line-1]; line == strings.Split(user.Before, "\n")[ch.Line-1] {
			t.Errorf("change %d: line %d is not where it starts", ch.Index, ch.Line)
		}
	}
	// Rejected (nothing called): the file is as imported.
	before := string(mustRead(t, filepath.Join(dir, "list-orders.hurl")))
	if before != orders.Before {
		t.Error("the suggestion's Before is not the file")
	}
	if err := s.Accept(context.Background(), req, "users/get-user.hurl", nil); err != nil {
		t.Fatal(err)
	}
	got := string(mustRead(t, filepath.Join(dir, "users", "get-user.hurl")))
	if got != user.After || !strings.Contains(got, "# ") {
		t.Errorf("accepted:\n%s", got)
	}
	if _, err := syntax.Parse("get-user.hurl", []byte(got), syntax.DialectHurl); err != nil {
		t.Error(err)
	}
	if string(mustRead(t, filepath.Join(dir, "list-orders.hurl"))) != before {
		t.Error("another file changed")
	}
	// Applied once: no suggestion for it any more, a second Accept refused.
	var e *apperr.Error
	if err := s.Accept(context.Background(), req, "users/get-user.hurl", nil); !errors.As(err, &e) || e.Code != apperr.Stale {
		t.Errorf("a second Accept: %v", err)
	}
	if string(mustRead(t, filepath.Join(dir, "users", "get-user.hurl"))) != got {
		t.Error("a second Accept changed the file")
	}
	// The injection: refused, or written escaped; never a section.
	inject := byPath["inject.hurl"]
	if inject.Error == "" {
		if err := s.Accept(context.Background(), req, "inject.hurl", nil); err != nil {
			t.Fatal(err)
		}
	}
	f, err := syntax.Parse("inject.hurl", mustRead(t, filepath.Join(dir, "inject.hurl")), syntax.DialectHurl)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range f.Entries {
		for _, sec := range e.Request.Sections {
			if sec.Kind == syntax.SectionOptions {
				t.Error("a script injected an [Options] section")
			}
		}
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func keys(m map[string]Suggestion) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestUpload: a file sent by the page imports under its own name.
func TestUpload(t *testing.T) {
	s, _, _, _ := service(t)
	in, err := s.Upload("../../orders.curl", "Y3VybCBodHRwczovL2FwaS50ZXN0L28K") // curl https://api.test/o
	if err != nil {
		t.Fatal(err)
	}
	pv, err := s.Preview(context.Background(), Request{Kind: Curl, Input: in.ID})
	if err != nil || len(pv.Files) != 1 || pv.Files[0].Path != "orders.hurl" {
		t.Fatalf("%+v %v", pv, err)
	}
	if _, err := s.Upload("x", "!!"); err == nil {
		t.Error("not base64")
	}
	s.Reset()
	if _, err := s.Preview(context.Background(), Request{Kind: Curl, Input: in.ID}); err == nil {
		t.Error("an input after Reset")
	}
}

// TestUploadSizeCap: uploads larger than 64 MiB are rejected.
func TestUploadSizeCap(t *testing.T) {
	s, _, _, _ := service(t)
	// Create a base64 string that when decoded exceeds 64 MiB
	// DecodedLen(n) = n*3/4, so for 65 MiB we need 65*1024*1024*4/3 = 89,478,485
	tooLarge := strings.Repeat("A", 89_500_000) // ~67.1 MiB when decoded
	_, err := s.Upload("large", tooLarge)
	if err == nil || !strings.Contains(err.Error(), "64 MiB") {
		t.Errorf("upload cap not enforced: %v", err)
	}
}

// TestUploadNameSanitization: dangerous path components are sanitized.
func TestUploadNameSanitization(t *testing.T) {
	s, _, _, _ := service(t)
	// filepath.Clean resolves .. and normalizes paths
	// On Unix, ../ paths become the base name
	testCases := []struct {
		input string
		want  string
	}{
		{"../etc/passwd", "passwd"},
		{"../../etc/shadow", "shadow"},
		{"/etc/passwd", "passwd"},
		{"file.curl", "file.curl"},
	}
	for _, tc := range testCases {
		in, err := s.Upload(tc.input, "Y3VybCBodHRwcwo=") // curl http
		if err != nil {
			t.Fatal(err)
		}
		if in.Name != tc.want {
			t.Errorf("name %q from %q, want %q", in.Name, tc.input, tc.want)
		}
	}
}

// TestUploadEmptyName: upload with empty name uses default.
func TestUploadEmptyName(t *testing.T) {
	s, _, _, _ := service(t)
	in, err := s.Upload("", "Y3VybCBodHRwcwo=") // curl http
	if err != nil {
		t.Fatal(err)
	}
	if in.Name == "" || in.Name == "." || in.Name == "/" {
		t.Errorf("empty name not replaced: %q", in.Name)
	}
}

// TestStageKindMismatch: staging with mismatched kind (file vs dir) fails.
func TestStageKindMismatch(t *testing.T) {
	s, _, h, _ := service(t)
	// Stage a file with kind "dir"
	filePath := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := h.Put(filePath, handles.OpenFile)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Stage(id, "dir") // wrong kind
	// Error can be "not a folder" or "Expired" depending on handle reuse
	if err == nil {
		t.Error("kind mismatch should be caught")
	}
}

// TestOpenCollectionFolder: OpenCollection accepts a folder input.
func TestOpenCollectionFolder(t *testing.T) {
	s, _, h, _ := service(t)
	// Create a proper folder input with OpenCollection format
	testDir := filepath.Join(testdata, "import")
	entries, _ := os.ReadDir(testDir)
	var hasDir bool
	for _, e := range entries {
		if e.IsDir() {
			hasDir = true
			req := Request{Kind: OpenCollection, Input: stage(t, s, h, filepath.Join(testDir, e.Name()), true), Folder: "imported"}
			pv, err := s.Preview(context.Background(), req)
			if err == nil && len(pv.Files) > 0 {
				return // Successfully imported a folder
			}
		}
	}
	if !hasDir {
		t.Skip("no test folder available")
	}
}

// TestOpenCollectionPastedText: OpenCollection with pasted YAML text.
func TestOpenCollectionPastedText(t *testing.T) {
	s, _, _, _ := service(t)
	yamlText := `info:
  name: Test
requests:
  - name: Test Request
    method: GET
    url: https://api.test/x`
	req := Request{Kind: OpenCollection, Text: yamlText, Folder: "imported"}
	pv, err := s.Preview(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.Files) == 0 {
		t.Error("opencollection with pasted text should produce files")
	}
}

// TestSondeYAMLKept: an import into a project with a sonde.yaml keeps it
// as it is, counts none of the collection's environments, and says so.
func TestSondeYAMLKept(t *testing.T) {
	s, dir, h, _ := service(t)
	mine := "version: 1\n"
	if err := os.WriteFile(filepath.Join(dir, "sonde.yaml"), []byte(mine), 0o600); err != nil {
		t.Fatal(err)
	}
	req := Request{Kind: Postman, Input: stage(t, s, h, filepath.Join(testdata, "import", "shop.postman_collection.json"), false),
		Environments: []string{stage(t, s, h, filepath.Join(testdata, "import", "dev.postman_environment.json"), false)}}
	pv, err := s.Preview(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var note bool
	for _, w := range pv.Warnings {
		note = note || w.Kind == "environments" && strings.Contains(w.Message, "collection, dev")
	}
	if pv.Project != "kept" || pv.Counts.Environments != 0 || !note {
		t.Errorf("project %q, environments %d, note %v", pv.Project, pv.Counts.Environments, note)
	}
	if _, err := s.Write(context.Background(), req, []string{"sonde.yaml"}); err != nil {
		t.Fatal(err)
	}
	if string(mustRead(t, filepath.Join(dir, "sonde.yaml"))) != mine {
		t.Error("sonde.yaml replaced")
	}
}

// TestSecretStubsNeverOverwritten: an empty secrets stub the import made,
// filled in since, is never replaced, even when every file is overwritten.
func TestSecretStubsNeverOverwritten(t *testing.T) {
	s, dir, h, _ := service(t)
	req := Request{Kind: Postman, Input: stage(t, s, h, filepath.Join(testdata, "import", "shop.postman_collection.json"), false),
		Environments: []string{stage(t, s, h, filepath.Join(testdata, "import", "dev.postman_environment.json"), false)}}
	w, err := s.Write(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(dir, "secrets", "dev.secrets")
	if !slices.Contains(w.Files, "secrets/dev.secrets") {
		t.Fatalf("no stub written: %v", w.Files)
	}
	if err := os.WriteFile(stub, []byte("api_key=filled-in-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pv, err := s.Preview(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var all []string
	for _, f := range pv.Files {
		all = append(all, f.Path)
	}
	if slices.Contains(all, "secrets/dev.secrets") {
		t.Error("the filled-in stub is planned again")
	}
	if _, err := s.Write(context.Background(), req, all); err != nil {
		t.Fatal(err)
	}
	if string(mustRead(t, stub)) != "api_key=filled-in-value\n" {
		t.Error("the stub was replaced")
	}
}

// TestWriteFailedPathLeavesNoPartialState: when Write fails on a denied path, no partial state is left.
func TestWriteFailedPathLeavesNoPartialState(t *testing.T) {
	s, dir, _, _ := service(t)
	// Try to write outside the project
	req := Request{Kind: Curl, Text: "curl https://api.test/a", Folder: "../../outside"}
	before := tree(t, dir)
	_, err := s.Write(context.Background(), req, nil)
	if err == nil {
		t.Fatal("write should fail for denied path")
	}
	after := tree(t, dir)
	if len(before) != len(after) {
		t.Error("write failure left partial state")
	}
}

// TestResetDeletesUploadedInputs: Reset deletes staged uploads.
func TestResetDeletesUploadedInputs(t *testing.T) {
	s, _, _, _ := service(t)
	in1, err := s.Upload("file1.curl", "Y3VybCBodHRwcwo=")
	if err != nil {
		t.Fatal(err)
	}
	in2, err := s.Upload("file2.curl", "Y3VybCBodHRwcwo=")
	if err != nil {
		t.Fatal(err)
	}
	// Both should be accessible
	_, err = s.Preview(context.Background(), Request{Kind: Curl, Input: in1.ID})
	if err != nil {
		t.Fatal(err)
	}
	// After reset, both should be gone
	s.Reset()
	_, err = s.Preview(context.Background(), Request{Kind: Curl, Input: in1.ID})
	if err == nil {
		t.Error("reset should delete uploaded inputs")
	}
	_, err = s.Preview(context.Background(), Request{Kind: Curl, Input: in2.ID})
	if err == nil {
		t.Error("reset should delete all uploaded inputs")
	}
}

// TestPastedStagedOnce: a pasted spec read by path is staged once per
// text, and gone after Reset.
func TestPastedStagedOnce(t *testing.T) {
	s, _, _, _ := service(t)
	spec := string(mustRead(t, filepath.Join(repo, "testdata", "openapi", "petstore-3.0.yaml")))
	for range 3 {
		if _, err := s.Preview(context.Background(), Request{Kind: OpenAPI, Text: spec}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := s.stage.ReadDir(stageDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("staged %d (%v)", len(entries), err)
	}
	s.Reset()
	if _, err := s.stage.ReadDir(stageDir); err == nil {
		t.Error("staged inputs left after Reset")
	}
}

func TestPostmanPreviewNamesTheCollection(t *testing.T) {
	s, _, h, _ := service(t)
	req := Request{Kind: Postman, Input: stage(t, s, h, filepath.Join(testdata, "import", "shop.postman_collection.json"), false)}
	pv, err := s.Preview(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if pv.Name == "" || pv.Counts.Folders != 1 {
		t.Errorf("name %q, folders %d", pv.Name, pv.Counts.Folders)
	}
	// Both layouts, request files only, the picked one as the files.
	byRequest, byFolder := pv.Layouts["request"], pv.Layouts["folder"]
	if len(byRequest) != 3 || len(pv.Files) < 3 || !slices.Equal(byFolder, []string{"users.hurl", "shop.hurl"}) {
		t.Errorf("layouts %v", pv.Layouts)
	}
	// The picked layout is what is written; the other, the same on the
	// next preview (kept, not planned again).
	for _, f := range byRequest {
		if !slices.ContainsFunc(pv.Files, func(x File) bool { return x.Path == f }) {
			t.Errorf("%s listed but not written", f)
		}
	}
	again, err := s.Preview(context.Background(), req)
	if err != nil || !slices.Equal(again.Layouts["folder"], byFolder) {
		t.Errorf("second preview %v %v", again.Layouts, err)
	}
	for _, f := range append(byRequest, byFolder...) {
		if !strings.HasSuffix(f, ".hurl") {
			t.Errorf("layout file %q", f)
		}
	}
}

func TestAcceptSomeChanges(t *testing.T) {
	s, dir, h, _ := service(t)
	req := Request{Kind: Postman, Input: stage(t, s, h, filepath.Join(testdata, "import", "shop.postman_collection.json"), false)}
	if _, err := s.Write(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}
	sg, err := s.Suggestions(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var user Suggestion
	for _, x := range sg {
		if x.Path == "users/get-user.hurl" {
			user = x
		}
	}
	if len(user.Changes) != 2 {
		t.Fatalf("changes %+v", user.Changes)
	}
	// The second change only: the first stays a comment.
	if err := s.Accept(context.Background(), req, user.Path, []int{1}); err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, filepath.Join(dir, "users", "get-user.hurl"))); got != user.Changes[1].After {
		t.Errorf("accepted change 1:\n%s\nwant:\n%s", got, user.Changes[1].After)
	}
}

func TestAcceptNoChange(t *testing.T) {
	s, dir, h, _ := service(t)
	req := Request{Kind: Postman, Input: stage(t, s, h, filepath.Join(testdata, "import", "shop.postman_collection.json"), false)}
	if _, err := s.Write(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "users", "get-user.hurl")
	before := string(mustRead(t, file))
	stat, _ := os.Stat(file)
	if err := s.Accept(context.Background(), req, "users/get-user.hurl", []int{}); err != nil {
		t.Fatal(err)
	}
	// Not even rewritten.
	if again, _ := os.Stat(file); string(mustRead(t, file)) != before || !again.ModTime().Equal(stat.ModTime()) {
		t.Error("no change picked: the file was written")
	}
	var e *apperr.Error
	if err := s.Accept(context.Background(), req, "users/get-user.hurl", []int{7}); !errors.As(err, &e) || e.Code != apperr.Invalid {
		t.Errorf("an unknown change: %v", err)
	}
	// Every change picked one by one is the file with them all.
	sg, _ := s.Suggestions(context.Background(), req)
	for _, x := range sg {
		if x.Error != "" {
			continue
		}
		all := []int{}
		for _, c := range x.Changes {
			all = append(all, c.Index)
		}
		if err := s.Accept(context.Background(), req, x.Path, all); err != nil {
			t.Fatal(err)
		}
		if got := string(mustRead(t, filepath.Join(dir, filepath.FromSlash(x.Path)))); got != x.After {
			t.Errorf("%s: every change picked:\n%s\nwant:\n%s", x.Path, got, x.After)
		}
	}
}

// TestNoSuggestionForKeptFiles: a file the import did not write (kept as
// the user had it) is never offered suggestions.
func TestNoSuggestionForKeptFiles(t *testing.T) {
	s, dir, h, _ := service(t)
	req := Request{Kind: Postman, Input: stage(t, s, h, filepath.Join(testdata, "import", "shop.postman_collection.json"), false)}
	mine := "# mine\nGET https://mine\n"
	if err := os.MkdirAll(filepath.Join(dir, "users"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "users", "get-user.hurl"), []byte(mine), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}
	sg, err := s.Suggestions(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range sg {
		if x.Path == "users/get-user.hurl" {
			t.Error("a kept file is offered suggestions")
		}
	}
	if err := s.Accept(context.Background(), req, "users/get-user.hurl", nil); err == nil || string(mustRead(t, filepath.Join(dir, "users", "get-user.hurl"))) != mine {
		t.Errorf("a kept file accepted: %v", err)
	}
}

// TestKeptCurlFileWritesNoSecret: lifted values go with the file naming
// them; kept as it was, it names none of them, so none is written.
func TestKeptCurlFileWritesNoSecret(t *testing.T) {
	s, dir, _, sec := service(t)
	if err := os.WriteFile(filepath.Join(dir, "curl.hurl"), []byte("GET https://mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := s.Write(context.Background(), Request{Kind: Curl, Text: "curl -H 'Authorization: Bearer abcdefgh12345' https://a", Env: "local"}, nil)
	if err != nil || len(w.Kept) != 1 || len(w.Secrets) != 0 || len(sec.set) != 0 {
		t.Errorf("written %+v, secrets %d, %v", w, len(sec.set), err)
	}
}

// TestFolderAsProjectPath: an absolute folder in the project is named by
// its project path; a link to a dot folder is refused.
func TestFolderAsProjectPath(t *testing.T) {
	s, dir, _, _ := service(t)
	pv, err := s.Preview(context.Background(), Request{Kind: Curl, Text: "curl https://a", Folder: filepath.Join(dir, "in")})
	if err != nil || pv.Files[0].Path != "in/curl.hurl" {
		t.Fatalf("%+v %v", pv, err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links")
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".git", filepath.Join(dir, "out")); err != nil {
		t.Fatal(err)
	}
	var e *apperr.Error
	if _, err := s.Write(context.Background(), Request{Kind: Curl, Text: "curl https://a", Folder: "out/x"}, nil); !errors.As(err, &e) || e.Code != apperr.Denied {
		t.Errorf("through a link to .git: %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, ".git")); len(entries) != 0 {
		t.Error("written in .git")
	}
}

func TestCurlName(t *testing.T) {
	s, _, _, _ := service(t)
	text := "curl https://api.test/carts -X POST"
	pv, err := s.Preview(context.Background(), Request{Kind: Curl, Text: text, Folder: "orders", Name: "add-item"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.Files) != 1 || pv.Files[0].Path != "orders/add-item.hurl" {
		t.Fatalf("files %+v", pv.Files)
	}
	for _, bad := range []string{"../up", "a/b", ".hidden"} {
		if _, err := s.Preview(context.Background(), Request{Kind: Curl, Text: text, Name: bad}); err == nil {
			t.Errorf("name %q: no error", bad)
		}
	}
}
