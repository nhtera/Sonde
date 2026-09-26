// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/internal/convert"
)

// TestImportHTTPStemFromStdin checks that INPUT "-" reads from stdin and
// names the single output file "requests".
func TestImportHTTPStemFromStdin(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringArray("env-file", nil, "")
	cmd.SetIn(strings.NewReader("GET https://example.test/x\n"))

	out, err := importHTTPFile(cmd, "-", convert.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 1 || out.Files[0].Path != "requests" {
		t.Fatalf("Files = %+v", out.Files)
	}
}

func TestImportHTTPStemFromFilename(t *testing.T) {
	path := writeTemp(t, "my-requests.http", "GET https://example.test/x\n")
	cmd := &cobra.Command{}
	cmd.Flags().StringArray("env-file", nil, "")

	out, err := importHTTPFile(cmd, path, convert.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 1 || out.Files[0].Path != "my-requests" {
		t.Fatalf("Files = %+v", out.Files)
	}
}

func TestImportHTTPRegistered(t *testing.T) {
	code, out, errOut := runArgs(t, "import", "--help")
	if code != ExitOK || !strings.Contains(out, "http") {
		t.Fatalf("import --help should list the http kind: exit %d\n%s%s", code, out, errOut)
	}
	dir := t.TempDir()
	src := writeTemp(t, "x.http", "GET https://example.test/x\n")
	code, _, errOut = runArgs(t, "import", "http", src, "-o", dir)
	if code != ExitOK {
		t.Fatalf("import: exit %d\n%s", code, errOut)
	}
}
