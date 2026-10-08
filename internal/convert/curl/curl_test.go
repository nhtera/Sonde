// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package curl

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

var update = flag.Bool("update", false, "rewrites the golden files")

// TestImportGolden imports every testdata/convert/curl/*.txt fixture and
// compares the generated file, its warnings and its skipped commands
// against the matching *.golden file (go test -run TestImportGolden
// -update rewrites them).
func TestImportGolden(t *testing.T) {
	matches, err := filepath.Glob("../../../testdata/convert/curl/*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("no fixtures found")
	}
	for _, path := range matches {
		name := strings.TrimSuffix(filepath.Base(path), ".txt")
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path) //nolint:gosec // G304: test fixture
			if err != nil {
				t.Fatal(err)
			}
			res, err := Import(data, syntax.DialectHurl)
			var b strings.Builder
			if err != nil {
				fmt.Fprintf(&b, "error: %v\n", err)
			} else {
				b.Write(syntax.Lint(res.File))
				for _, w := range res.Warnings {
					fmt.Fprintf(&b, "warning %s: %s\n", w.Kind, w.Message)
				}
				for _, s := range res.Skipped {
					fmt.Fprintf(&b, "skipped %s: %s\n", s.Name, s.Reason)
				}
			}

			golden := filepath.Join("../../../testdata/convert/curl", name+".golden")
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
