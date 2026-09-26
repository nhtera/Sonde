// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package opencollection

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

// FuzzOpenCollection feeds arbitrary bytes to ImportFile: it must never
// panic, regardless of how malformed the input is
// (docs/guides/import-export.md §Security Considerations).
func FuzzOpenCollection(f *testing.F) {
	// filepath.Glob has no recursive "**"; list the fixture tree's few
	// depths explicitly (docs/decisions/0002-opencollection-mapping.md's
	// "Directory" layout nests at most collection/folder/file).
	for _, pattern := range []string{"*/*.yml", "*/*/*.yml", "*/*/*/*.yml"} {
		seeds, _ := filepath.Glob("../../../testdata/convert/opencollection/" + pattern)
		for _, s := range seeds {
			if data, err := os.ReadFile(s); err == nil { //nolint:gosec // G304: fixed test corpus glob
				f.Add(data)
			}
		}
	}
	for _, s := range []string{
		"", "{}", "null", "[]", "opencollection", "opencollection: 1\n",
		"items: [1, 2, 3]\n",
		"items:\n  - info: {name: x, type: http}\n    http: {method: GET, url: \"{{x\"}\n",
		"items:\n  - &a {info: {name: x, type: folder}, items: [*a]}\n",
		"config:\n  environments:\n    - name: x\n      variables:\n        - {name: y, secret: true, value: leaked}\n",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		out, err := ImportFile(data, syntax.DialectHurl)
		if err != nil {
			return
		}
		for _, gf := range out.Files {
			if _, err := syntax.Parse(gf.Path, syntax.Format(gf.File), syntax.DialectHurl); err != nil {
				t.Errorf("generated file %s does not parse: %v", gf.Path, err)
			}
		}
	})
}
