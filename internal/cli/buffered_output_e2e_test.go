// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"os"
	"strings"
	"testing"
)

// TestE2EBufferedModeOutputDashGoesToStdout checks code-reviewer finding
// #17: an entry's own `output: -` always writes to the process's real
// stdout, the same as sequential mode, even when -o FILE is also given and
// the run uses the parallel/buffered runner (--parallel or --test). Only
// the CLI's own default last-response-body output (with no entry-level
// `output:` of its own) goes to -o FILE.
func TestE2EBufferedModeOutputDashGoesToStdout(t *testing.T) {
	srv := testServer(t)
	file := writeTemp(t, "ok.hurl",
		"GET "+srv.URL+"/hello\n[Options]\noutput: -\nHTTP 200\n\n"+
			"GET "+srv.URL+"/json\nHTTP 200\n")
	outFile := writeTemp(t, "out.json", "")

	// --parallel (not --test, which would also set --no-output) with
	// --jobs 1 still selects the parallel/buffered runner (rc.parallel),
	// exercising the buffered code path this finding is about.
	code, out, errOut := runArgs(t, "--parallel", "--jobs", "1", "-o", outFile, file)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ExitOK, errOut)
	}

	if !strings.Contains(out, "Hello World!") {
		t.Errorf("stdout missing the entry's own `output: -` body:\n%q", out)
	}
	if strings.Contains(out, `{"a":1}`) {
		t.Errorf("stdout has the CLI's own default output, want it only in -o FILE:\n%q", out)
	}

	fileContent, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fileContent), `{"a":1}`) {
		t.Errorf("-o FILE missing the CLI's own default output:\n%q", fileContent)
	}
	if strings.Contains(string(fileContent), "Hello World!") {
		t.Errorf("-o FILE has the entry's own `output: -` body, want it only on stdout:\n%q", fileContent)
	}
}
