// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/exchange"
)

// TestWriteJSON_Cumulative reproduces
// testdata/conformance/hurl/tests_ok/report_json/report_json.sh: a first
// run of two files, then a second run of a third — report.json ends up
// holding all three, in order, across the two invocations.
func TestWriteJSON_Cumulative(t *testing.T) {
	dir := t.TempDir()

	run1 := []*engine.UnitResult{
		successResult("tests/test.1.hurl"),
		failureResult("tests/test.2.hurl"),
	}
	if err := WriteJSON(dir, run1, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	run2 := []*engine.UnitResult{successResult("tests/test.3.hurl")}
	if err := WriteJSON(dir, run2, redactTestSecret); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var results []Result
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatalf("report.json is not valid JSON: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	want := []string{"tests/test.1.hurl", "tests/test.2.hurl", "tests/test.3.hurl"}
	for i, w := range want {
		if results[i].Filename != w {
			t.Errorf("results[%d].Filename = %q, want %q", i, results[i].Filename, w)
		}
	}

	// Every response with a body was saved under store/ and referenced
	// from there.
	storeDir := filepath.Join(dir, "store")
	entries, err := os.ReadDir(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 { // one response body per result across both runs
		t.Errorf("got %d files under store/, want 3: %v", len(entries), entries)
	}
	for _, res := range results {
		for _, e := range res.Entries {
			for _, c := range e.Calls {
				if c.Response.Body != "" && !strings.HasPrefix(c.Response.Body, "store/") {
					t.Errorf("response body %q does not reference store/", c.Response.Body)
				}
			}
		}
	}
}

// TestWriteJSON_ResponseBodyKeyOrder checks the reference CLI's own
// alphabetical field order (see Result's doc comment): with a store
// present, "body" sorts before "cookies" and must be the first key of a
// "response" object — this is what testdata/conformance/hurl's
// tests_ok/report_json oracle byte-compares against.
func TestWriteJSON_ResponseBodyKeyOrder(t *testing.T) {
	dir := t.TempDir()
	if err := WriteJSON(dir, []*engine.UnitResult{successResult("t.hurl")}, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"response":{"body":"store/`) {
		t.Errorf(`expected a "response" object with "body" as its first key, got:\n%s`, data)
	}
}

// TestWriteJSON_Redacts checks the run's secret never survives into
// report.json.
func TestWriteJSON_Redacts(t *testing.T) {
	dir := t.TempDir()
	results := []*engine.UnitResult{successResult("t.hurl"), failureResult("t2.hurl")}
	if err := WriteJSON(dir, results, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), testSecret) {
		t.Errorf("secret leaked into report.json:\n%s", data)
	}
}

// TestWriteJSON_RedactsResponseBodyStore is the regression test for
// review finding #3 (2026-09-26): --report-json used to save response
// bodies to store/ completely unredacted, contradicting
// docs/report-json.md. successResult's fixture already embeds the run's
// secret in its response body ("hello, " + testSecret); it must not
// survive on disk.
func TestWriteJSON_RedactsResponseBodyStore(t *testing.T) {
	dir := t.TempDir()
	if err := WriteJSON(dir, []*engine.UnitResult{successResult("t.hurl")}, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no response body was saved under store/")
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, "store", e.Name())) //nolint:gosec // G304: e.Name() from os.ReadDir over a t.TempDir()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), testSecret) {
			t.Errorf("secret leaked into stored response body %s:\n%s", e.Name(), data)
		}
	}
}

// TestWriteJSON_RedactsBinaryResponseBody checks redaction is applied to
// a stored body regardless of whether it is text: the secret's raw bytes
// are still an exact byte sequence to find and mask inside binary data,
// and the surrounding bytes must otherwise survive untouched.
func TestWriteJSON_RedactsBinaryResponseBody(t *testing.T) {
	dir := t.TempDir()
	res := successResult("bin.hurl")
	body := append([]byte{0x00, 0x01, 0xff, 0xfe}, []byte(testSecret)...)
	body = append(body, 0x02, 0x03)
	res.Entries[0].Calls[0].Response.Body = body
	res.Entries[0].Calls[0].Response.Headers = exchange.Headers{{Name: "Content-Type", Value: "application/octet-stream"}}

	if err := WriteJSON(dir, []*engine.UnitResult{res}, redactTestSecret); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d files under store/, want 1", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(dir, "store", entries[0].Name())) //nolint:gosec // G304: entries[0].Name() from os.ReadDir over a t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), testSecret) {
		t.Errorf("secret leaked into stored binary response body:\n%x", data)
	}
	want := append(append([]byte{0x00, 0x01, 0xff, 0xfe}, []byte("***")...), 0x02, 0x03)
	if string(data) != string(want) {
		t.Errorf("stored body = %x, want %x (surrounding bytes must survive untouched)", data, want)
	}
}

// TestWriteJSON_CumulativePreservesExistingEntries is the regression test
// for review finding #6 (2026-09-26): report.json's existing entries used
// to be decoded into []Result (Capture.Value: any) and re-encoded on
// every write, which silently corrupted a big integer's precision, an
// object's member order and any field Result does not model — including
// the reserved "sonde" key. WriteJSON must instead splice a pre-existing
// entry back in byte-for-byte, decoding only the entries it is adding.
func TestWriteJSON_CumulativePreservesExistingEntries(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.json")
	if err := os.MkdirAll(filepath.Join(dir, "store"), 0o750); err != nil {
		t.Fatal(err)
	}

	// A hand-written "existing" entry, as if from a prior run (possibly
	// even upstream's own CLI): a big integer past float64's exact range,
	// an object whose member order is not alphabetical, and a "sonde" key
	// this package's Result type has no field for at all.
	const existing = `[{"filename":"prior.hurl","success":true,"time":1,"cookies":[],` +
		`"entries":[{"index":1,"line":1,"time":0,"curl_cmd":"curl",` +
		`"captures":[{"name":"big","value":12345678901234567891},` +
		`{"name":"obj","value":{"z":1,"a":2}}],"asserts":[],"calls":[]}],` +
		`"sonde":{"future":"field"}}]`
	if err := os.WriteFile(reportPath, []byte(existing), 0o644); err != nil { //nolint:gosec // G306: test fixture
		t.Fatal(err)
	}

	if err := WriteJSON(dir, []*engine.UnitResult{successResult("new.hurl")}, redactTestSecret); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("report.json is not valid JSON: %v", err)
	}
	if len(raw) != 2 {
		t.Fatalf("got %d entries, want 2", len(raw))
	}
	got := strings.TrimSpace(string(raw[0]))
	if got != existing[1:len(existing)-1] {
		t.Errorf("existing entry was rewritten:\ngot:  %s\nwant: %s", got, existing[1:len(existing)-1])
	}
	for _, want := range []string{`12345678901234567891`, `{"z":1,"a":2}`, `"sonde":{"future":"field"}`} {
		if !strings.Contains(got, want) {
			t.Errorf("existing entry lost %q:\n%s", want, got)
		}
	}
}

func TestBodyExtension(t *testing.T) {
	cases := map[string]string{
		"application/json":                ".json",
		"application/json; charset=utf-8": ".json",
		"application/vnd.api+json":        ".json",
		"text/xml":                        ".xml",
		"application/xml":                 ".xml",
		"application/atom+xml":            ".xml",
		"text/html; charset=utf-8":        ".html",
		"text/plain":                      "",
		"application/octet-stream":        "",
		"":                                "",
	}
	for ct, want := range cases {
		if got := bodyExtension(ct); got != want {
			t.Errorf("bodyExtension(%q) = %q, want %q", ct, got, want)
		}
	}
}
