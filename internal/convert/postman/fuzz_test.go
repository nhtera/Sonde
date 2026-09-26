// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package postman

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

// FuzzPostman feeds arbitrary bytes to Import: it must never panic,
// regardless of how malformed the input is (docs/guides/import-export.md
// §Security Considerations).
func FuzzPostman(f *testing.F) {
	seeds, _ := filepath.Glob("../../../testdata/convert/postman/*.json")
	for _, s := range seeds {
		if data, err := os.ReadFile(s); err == nil { //nolint:gosec // G304: fixed test corpus glob
			f.Add(data)
		}
	}
	for _, s := range []string{
		`{}`, `[]`, `null`, `"x"`, `123`,
		`{"info":{"name":"x"},"item":[]}`,
		`{"info":{"name":"x"},"requests":[]}`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, group := range []string{"", GroupRequest, GroupFolder} {
			out, err := Import(data, Options{Group: group, Dialect: syntax.DialectHurl})
			if err != nil {
				continue
			}
			for _, gf := range out.Files {
				if _, err := syntax.Parse(gf.Path, syntax.Format(gf.File), syntax.DialectHurl); err != nil {
					t.Errorf("generated file %s does not parse: %v", gf.Path, err)
				}
			}
		}
	})
}
