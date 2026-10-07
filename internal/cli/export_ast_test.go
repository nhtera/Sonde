// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportJSONAndHTML(t *testing.T) {
	path := writeTemp(t, "a.hurl", "GET http://a/<b>\nHTTP 200\n")

	code, out, _ := runArgs(t, "export", "json", path)
	want := `{"entries":[{"request":{"method":"GET","url":"http://a/<b>"},"response":{"status":200}}]}` + "\n"
	if code != ExitOK || out != want || !json.Valid([]byte(out)) {
		t.Errorf("json: code %d, stdout %q, want %q", code, out, want)
	}

	code, out, _ = runArgs(t, "export", "html", path)
	if code != ExitOK || !strings.HasPrefix(out, `<pre><code class="language-hurl"><span class="entry">`) ||
		!strings.Contains(out, `<span class="url">http://a/&lt;b&gt;</span>`) || !strings.HasSuffix(out, "</code></pre>\n") {
		t.Errorf("html: code %d, stdout %q", code, out)
	}

	code, out, _ = runArgs(t, "export", "html", "--standalone", path)
	if code != ExitOK || !strings.HasPrefix(out, "<!DOCTYPE html>") || !strings.Contains(out, "<style>") {
		t.Errorf("html --standalone: code %d, stdout %q", code, out)
	}
}

func TestExportJSONStdinAndOutputFile(t *testing.T) {
	withStdin(t, "GET http://a\n")
	dest := filepath.Join(t.TempDir(), "out.json")
	if code, out, _ := runArgs(t, "export", "json", "-o", dest); code != ExitOK || out != "" {
		t.Fatalf("code %d, stdout %q", code, out)
	}
	b, err := os.ReadFile(dest) //nolint:gosec // G304: test temp file.
	if want := `{"entries":[{"request":{"method":"GET","url":"http://a"}}]}` + "\n"; err != nil || string(b) != want {
		t.Errorf("output file = %q, %v; want %q", b, err, want)
	}
}

func TestExportASTErrors(t *testing.T) {
	sonde := writeTemp(t, "a.sonde", "GET http://a\n")
	if code, _, errOut := runArgs(t, "export", "json", sonde); code != ExitUsage || !strings.Contains(errOut, "supports .hurl files only") {
		t.Errorf(".sonde: code %d, stderr %q", code, errOut)
	}
	good := writeTemp(t, "good.hurl", "GET http://a\n")
	bad := writeTemp(t, "bad.hurl", "xxx\n")
	code, out, errOut := runArgs(t, "export", "html", good, bad)
	if code != ExitParse || !strings.Contains(errOut, "Parsing method") || !strings.Contains(out, `<span class="method">GET</span>`) {
		t.Errorf("parse error: code %d, stdout %q, stderr %q; want the good file exported and exit 2", code, out, errOut)
	}
}
