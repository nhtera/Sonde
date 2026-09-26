// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpfile

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

var update = flag.Bool("update", false, "rewrites the golden files")

// TestImportGolden imports every testdata/convert/httpfile/*.http fixture
// and compares the generated file, sonde.yaml, extra files and warnings
// with testdata/convert/httpfile/<fixture>.golden.
func TestImportGolden(t *testing.T) {
	for _, name := range []string{"jetbrains.http", "restclient.http"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("../../../testdata/convert/httpfile", name)) //nolint:gosec // G304: fixed test fixture path
			if err != nil {
				t.Fatal(err)
			}
			out, err := Import(strings.TrimSuffix(name, ".http"), data, syntax.DialectHurl, Options{})
			if err != nil {
				t.Fatal(err)
			}
			var b strings.Builder
			for _, f := range out.Files {
				rendered := syntax.Format(f.File)
				if _, err := syntax.Parse(f.Path+".hurl", rendered, syntax.DialectHurl); err != nil {
					t.Errorf("%s does not parse: %v", f.Path, err)
				}
				fmt.Fprintf(&b, "== %s\n%s", f.Path, rendered)
			}
			if out.ProjectYAML != nil {
				fmt.Fprintf(&b, "== sonde.yaml\n%s", out.ProjectYAML)
			}
			for _, extra := range out.Extra {
				fmt.Fprintf(&b, "== %s\n%s\n", extra.Path, extra.Data)
			}
			warns := append([]convert.Warning(nil), out.Warnings...)
			sort.Slice(warns, func(i, j int) bool {
				if warns[i].Kind != warns[j].Kind {
					return warns[i].Kind < warns[j].Kind
				}
				return warns[i].Message < warns[j].Message
			})
			for _, w := range warns {
				fmt.Fprintf(&b, "warning %s: %s\n", w.Kind, w.Message)
			}

			golden := filepath.Join("../../../testdata/convert/httpfile", strings.TrimSuffix(name, ".http")+".golden")
			if *update {
				if err := os.WriteFile(golden, []byte(b.String()), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden) //nolint:gosec // G304: test fixture
			if err != nil {
				t.Fatal(err)
			}
			if b.String() != string(want) {
				t.Errorf("output differs from %s (go test -run TestImportGolden -update):\n%s", golden, b.String())
			}
		})
	}
}
