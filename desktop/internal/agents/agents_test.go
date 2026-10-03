// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package agents

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/desktop/internal/sandboxtest"
	"github.com/nhtera/sonde/internal/sandbox"
)

func service(t *testing.T) (*Agents, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "my project")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	root := sandboxtest.Open(t, dir)
	return New(func() *sandbox.Root { return root }), root.Dir()
}

func codeOf(err error) string {
	if e, ok := errors.AsType[*apperr.Error](err); ok {
		return e.Code
	}
	return ""
}

// TestSnippets: each client's format and file, --root always set (the
// path on disk for a user's configuration, the workspace for one in the
// repository), run
// options only when running is allowed, the tools of each.
func TestSnippets(t *testing.T) {
	a, dir := service(t)
	s, err := a.Snippet(Request{Client: Claude, Scope: User, AllowRun: true, Hosts: []string{"api.test", " *.shop.dev "}})
	if err != nil {
		t.Fatal(err)
	}
	if want := "claude mcp add --scope user sonde -- sonde mcp --root '" + dir + "' --allow-run --allow-host api.test --allow-host '*.shop.dev'"; s.Text != want {
		t.Errorf("claude user:\n%s\nwant\n%s", s.Text, want)
	}
	if len(s.Tools) != 3 || s.Tools[2].Name != "sonde_run" {
		t.Errorf("tools %+v", s.Tools)
	}
	for _, c := range []struct{ client, scope, key, where, root string }{
		{Claude, Project, "mcpServers", ".mcp.json in the project folder", "."},
		{Cursor, Project, "mcpServers", ".cursor/mcp.json in the project folder", "${workspaceFolder}"},
		{Cursor, User, "mcpServers", "~/.cursor/mcp.json", dir},
		{VSCode, Project, "servers", ".vscode/mcp.json in the project folder", "${workspaceFolder}"},
	} {
		s, err := a.Snippet(Request{Client: c.client, Scope: c.scope})
		if err != nil {
			t.Fatalf("%s %s: %v", c.client, c.scope, err)
		}
		var v map[string]map[string]struct {
			Type    string   `json:"type"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
		}
		if err := json.Unmarshal([]byte(s.Text), &v); err != nil {
			t.Fatalf("%s: %v\n%s", c.client, err, s.Text)
		}
		srv := v[c.key]["sonde"]
		if srv.Command != "sonde" || strings.Join(srv.Args, " ") != "mcp --root "+c.root || s.Where != c.where {
			t.Errorf("%s %s: %+v %q", c.client, c.scope, srv, s.Where)
		}
		if (c.client == Cursor) != (srv.Type == "") {
			t.Errorf("%s: type %q", c.client, srv.Type)
		}
		if len(s.Tools) != 2 {
			t.Errorf("read-only tools %+v", s.Tools)
		}
	}
}

// TestRefused: every host in a shared configuration, a bad host, running
// without a host, an unknown client or scope, no project.
func TestRefused(t *testing.T) {
	a, _ := service(t)
	for name, req := range map[string]Request{
		"star in project": {Client: Cursor, Scope: Project, AllowRun: true, Hosts: []string{"*"}},
		"bad host":        {Client: Cursor, Scope: User, AllowRun: true, Hosts: []string{"http://x/y"}},
		"no host":         {Client: Cursor, Scope: User, AllowRun: true, Hosts: []string{" "}},
		"client":          {Client: "emacs", Scope: User},
		"scope":           {Client: Cursor, Scope: "team"},
	} {
		if _, err := a.Snippet(req); codeOf(err) != apperr.Invalid {
			t.Errorf("%s: %v", name, err)
		}
	}
	if s, err := a.Snippet(Request{Client: Cursor, Scope: User, AllowRun: true, Hosts: []string{"*"}}); err != nil || !strings.Contains(s.Text, `"*"`) {
		t.Errorf("star in user scope: %v", err)
	}
	none := New(func() *sandbox.Root { return nil })
	if _, err := none.Snippet(Request{Client: Cursor, Scope: User}); codeOf(err) != apperr.NotFound {
		t.Errorf("no project: %v", err)
	}
}

// TestShellJoin: shell quoting with single quotes and single-quote escaping;
// empty strings, spaces, special characters all quoted; safe strings unquoted.
func TestShellJoin(t *testing.T) {
	for name, test := range map[string]struct {
		args []string
		want string
	}{
		"safe":     {[]string{"foo", "bar"}, "foo bar"},
		"unsafe":   {[]string{"foo bar", "baz"}, "'foo bar' baz"},
		"empty":    {[]string{""}, "''"},
		"quote":    {[]string{"it's"}, "'it'\\''s'"},
		"multi":    {[]string{"a b", "c'd", "e"}, "'a b' 'c'\\''d' e"},
		"dash":     {[]string{"-f", "--flag=value"}, "-f --flag=value"},
		"slash":    {[]string{"/usr/bin", "path.txt", "@host.com"}, "/usr/bin path.txt @host.com"},
		"comma":    {[]string{"a,b", "c=d,e=f"}, "a,b c=d,e=f"},
		"dollar":   {[]string{"$var", "test$"}, "'$var' 'test$'"},
		"backtick": {[]string{"`cmd`"}, "'`cmd`'"},
		"zero":     {[]string{}, ""},
	} {
		if got := shellJoin(test.args); got != test.want {
			t.Errorf("%s: got %q, want %q", name, got, test.want)
		}
	}
}

// TestUnsafeInShell: rune classification for POSIX shell safety.
func TestUnsafeInShell(t *testing.T) {
	for name, char := range map[string]rune{
		"letter":     'a',
		"number":     '5',
		"dash":       '-',
		"underscore": '_',
		"slash":      '/',
		"at":         '@',
		"comma":      ',',
		"equals":     '=',
		"plus":       '+',
		"colon":      ':',
		"dot":        '.',
	} {
		if unsafeInShell(char) {
			t.Errorf("%s (%q) marked unsafe", name, char)
		}
	}
	for name, char := range map[string]rune{
		"space":     ' ',
		"tab":       '\t',
		"newline":   '\n',
		"dollar":    '$',
		"backquote": '`',
		"semicolon": ';',
		"ampersand": '&',
		"pipe":      '|',
		"less":      '<',
		"greater":   '>',
		"paren":     '(',
		"quote":     '"',
		"single":    '\'',
		"backslash": '\\',
	} {
		if !unsafeInShell(char) {
			t.Errorf("%s (%q) marked safe", name, char)
		}
	}
}
