// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package postman

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

var update = flag.Bool("update", false, "rewrites the golden files")

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../../../testdata/convert/postman", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestImportGolden(t *testing.T) {
	for _, tc := range []struct {
		name  string
		file  string
		group string
		envs  []string
	}{
		{"basic-request", "basic.json", GroupRequest, nil},
		{"basic-folder", "basic.json", GroupFolder, nil},
		{"basic-environments", "basic.json", GroupRequest, []string{"env/dev.postman_environment.json", "env/prod.postman_environment.json"}},
		{"graphql", "graphql.json", GroupRequest, nil},
		{"auth-types", "auth-types.json", GroupRequest, nil},
		{"folder-group", "folder-group.json", GroupFolder, nil},
		{"v2-compat", "v2-compat.json", GroupRequest, nil},
		{"path-variables", "path-variables.json", GroupRequest, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := Options{Group: tc.group, Dialect: syntax.DialectHurl}
			for _, e := range tc.envs {
				opts.Environments = append(opts.Environments, EnvironmentFile{FileName: e, Data: readFixture(t, e)})
			}
			out, err := Import(readFixture(t, tc.file), opts)
			if err != nil {
				t.Fatal(err)
			}
			got := renderOutput(t, out)
			golden := filepath.Join("../../../testdata/convert/postman", tc.name+".golden")
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden) //nolint:gosec // G304: fixed test golden path
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("%s: golden mismatch (-want +got not shown); run with -update\n--- got ---\n%s", tc.name, got)
			}
		})
	}
}

// TestInvalidMethodIsSkipped checks that a crafted method (an attempt to
// smuggle a second request past BuildFile's "never hand-assembled" source)
// becomes a Skipped item, and never a second entry in the output.
func TestInvalidMethodIsSkipped(t *testing.T) {
	col := `{"info":{"name":"x"},"item":[
		{"name":"smuggle","request":{"method":"GET {{base_url}}/evil\nGET","url":"{{base_url}}/legit"}},
		{"name":"ok","request":{"method":"GET","url":"{{base_url}}/ok"}}
	]}`
	out, err := Import([]byte(col), Options{Dialect: syntax.DialectHurl})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("files = %d, want 1 (the smuggled request must not produce a file)", len(out.Files))
	}
	found := false
	for _, s := range out.Skipped {
		if s.Name == "smuggle" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected \"smuggle\" to be skipped with a reason: %v", out.Skipped)
	}
}

// TestFolderModeSkipsOnlyTheBadEntry checks that one request with an
// invalid method does not sink the whole --group folder file: every other
// request of the same folder still ends up in the combined output.
func TestFolderModeSkipsOnlyTheBadEntry(t *testing.T) {
	col := `{"info":{"name":"x"},"item":[
		{"name":"a","request":{"method":"GET","url":"{{base_url}}/a"}},
		{"name":"bad","request":{"method":"GET {{base_url}}/evil\nGET","url":"{{base_url}}/legit"}},
		{"name":"b","request":{"method":"GET","url":"{{base_url}}/b"}}
	]}`
	out, err := Import([]byte(col), Options{Group: GroupFolder, Dialect: syntax.DialectHurl})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(out.Files))
	}
	src := string(syntax.Format(out.Files[0].File))
	if !strings.Contains(src, "/a") || !strings.Contains(src, "/b") {
		t.Errorf("the good requests should still be chained:\n%s", src)
	}
	if strings.Contains(src, "/evil") {
		t.Errorf("the smuggled method should never appear:\n%s", src)
	}
	found := false
	for _, s := range out.Skipped {
		if s.Name == "bad" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected \"bad\" to be skipped: %v", out.Skipped)
	}
}

// TestSlashInANameIsNoFolder checks that a slash in a folder or request
// name stays in its file's name, in both layouts.
func TestSlashInANameIsNoFolder(t *testing.T) {
	col := `{"info":{"name":"a/b"},"item":[
		{"name":"Fields / v2","item":[{"name":"Enable / disable field","request":{"method":"PATCH","url":"{{base_url}}/f"}}]},
		{"name":"top","request":{"method":"GET","url":"{{base_url}}/t"}}
	]}`
	for group, want := range map[string][]string{
		GroupRequest: {"Fields   v2/Enable   disable field", "top"},
		GroupFolder:  {"Fields   v2", "a b"},
	} {
		out, err := Import([]byte(col), Options{Group: group, Dialect: syntax.DialectHurl})
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, f := range out.Files {
			got = append(got, f.Path)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: paths %q, want %q", group, got, want)
		}
	}
}

// TestVariableNameCollisionWarns checks that two different collection
// variable names sanitizing to the same Sonde name (VariableName) produce a
// warning naming both, rather than one silently overwriting the other with
// no explanation.
func TestVariableNameCollisionWarns(t *testing.T) {
	col := `{"info":{"name":"x"},"variable":[
		{"key":"api.key","value":"one"},
		{"key":"api_key","value":"two"}
	],"item":[]}`
	out, err := Import([]byte(col), Options{Dialect: syntax.DialectHurl})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range out.Warnings {
		if w.Kind == convert.WarnUnsupported && strings.Contains(w.Message, "api.key") && strings.Contains(w.Message, "api_key") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a collision warning naming both variables: %v", out.Warnings)
	}
}

// TestPathVariableNameTakenByEnvironment checks that a path variable whose
// name an --environment file also defines keeps its own value inline,
// rather than becoming {{id}} and silently sending the environment's value.
func TestPathVariableNameTakenByEnvironment(t *testing.T) {
	col := `{"info":{"name":"x"},"item":[
		{"name":"get","request":{"method":"GET","url":{"raw":"{{base_url}}/orders/:id","variable":[{"key":"id","value":"ord_1"}]}}}
	]}`
	env := `{"name":"dev","values":[{"key":"id","value":"something-else"}]}`
	out, err := Import([]byte(col), Options{
		Dialect:      syntax.DialectHurl,
		Environments: []EnvironmentFile{{FileName: "dev.json", Data: []byte(env)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	src := string(syntax.Format(out.Files[0].File))
	if !strings.Contains(src, "/orders/ord_1") {
		t.Errorf("the path variable should keep its own value inline:\n%s", src)
	}
}

// renderOutput dumps every part of out deterministically, so a golden diff
// catches any change: request files (in the order Import produced them,
// which every case above keeps stable), extra files, the sonde.yaml
// skeleton, skipped items and warnings.
func renderOutput(t *testing.T, out convert.Output) string {
	t.Helper()
	var b strings.Builder
	for _, f := range out.Files {
		src := syntax.Format(f.File)
		if _, err := syntax.Parse(f.Path, src, syntax.DialectHurl); err != nil {
			t.Errorf("%s does not parse: %v", f.Path, err)
		}
		fmt.Fprintf(&b, "== %s\n%s", f.Path, src)
	}
	for _, x := range out.Extra {
		fmt.Fprintf(&b, "== extra %s\n%s", x.Path, x.Data)
	}
	if out.ProjectYAML != nil {
		fmt.Fprintf(&b, "== sonde.yaml\n%s", out.ProjectYAML)
	}
	for _, s := range out.Skipped {
		fmt.Fprintf(&b, "skipped %s: %s\n", s.Name, s.Reason)
	}
	for _, w := range out.Warnings {
		fmt.Fprintf(&b, "warning %s: %s\n", w.Kind, w.Message)
	}
	return b.String()
}
