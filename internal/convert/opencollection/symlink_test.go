// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package opencollection

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

// TestImportDirSymlinkEscapeRefused checks that a symlink inside a
// collection directory pointing outside it cannot make ImportDir read
// content from outside the directory (mapping doc, "Input layouts":
// "Traversal is done through os.OpenRoot(dir).FS() ... so a symlink
// inside the directory cannot walk the import outside it").
func TestImportDirSymlinkEscapeRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated privileges on windows")
	}
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(outside, 0o750); err != nil {
		t.Fatal(err)
	}
	secret := "leaked-secret-value"
	if err := os.WriteFile(filepath.Join(outside, "secret.yml"),
		[]byte("info: {name: Leak, type: http}\nhttp: {method: GET, url: \"http://x/"+secret+"\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(base, "collection")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.yml"), filepath.Join(dir, "escape.yml")); err != nil {
		t.Fatal(err)
	}

	out, err := ImportDir(dir, syntax.DialectHurl)
	if err == nil {
		// os.Root refused to follow the symlink; make sure nothing it
		// might have skipped still leaked the outside file's content.
		for _, f := range out.Files {
			if strings.Contains(string(syntax.Format(f.File)), secret) {
				t.Fatalf("%s: leaked content from outside the collection directory", f.Path)
			}
		}
		return
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked content from outside the collection directory: %v", err)
	}
}
