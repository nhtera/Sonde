// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

// FuzzHTTPFile checks that Import never panics on arbitrary input, for
// either output dialect.
func FuzzHTTPFile(f *testing.F) {
	dir := "../../../testdata/convert/httpfile"
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			if filepath.Ext(e.Name()) != ".http" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // G304: fixed seed corpus directory
			if err == nil {
				f.Add(data)
			}
		}
	}
	for _, seed := range []string{
		"", "###", "@=", "GET", "< {%", "> {%%}", ">>!",
		"POST x\nContent-Type: multipart/form-data; boundary=B\n\n--B\n--B--\n",
		"POST x\nContent-Type: application/json\n\n{\"a\": {{b}}}\n",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(_ *testing.T, data []byte) {
		for _, d := range []syntax.Dialect{syntax.DialectHurl, syntax.DialectSonde} {
			_, _ = Import("fuzz", data, d, Options{})
		}
	})
}
