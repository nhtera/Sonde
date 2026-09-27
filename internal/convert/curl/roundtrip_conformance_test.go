// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package curl

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/syntax"
)

// excludedOptionNames are [Options] this package's importer cannot express
// (or, for "skip"/"repeat", that mean the entry sends no request at all)
// with no per-entry curl equivalent: an entry that sets one of these is not
// a roundtrip failure, since RenderCurl (or curl.Import) never had a
// chance to preserve it in the first place.
var excludedOptionNames = map[string]string{
	"aws-sigv4":      "aws-sigv4: no --aws-sigv4 mapping (engine.unsupportedOptionErr territory)",
	"netrc":          "netrc: reads local credentials, out of scope for an importer",
	"netrc-optional": "netrc-optional: reads local credentials, out of scope for an importer",
	"netrc-file":     "netrc-file: reads local credentials, out of scope for an importer",
	"max-filesize":   "max-filesize: no per-entry [Options] curl equivalent exists in the grammar",
	"delay":          "delay: a run-timing option with no curl equivalent",
	"retry":          "retry: a run-timing option with no curl equivalent",
	"retry-interval": "retry-interval: a run-timing option with no curl equivalent",
	"repeat":         "repeat: RenderCurl sends nothing for repeat: 0, and repeats aren't curl-expressible otherwise",
	"skip":           "skip: RenderCurl sends nothing for a skipped entry",
}

// TestRoundtripConformance is the phase's success criterion: for every
// entry of every parseable *.hurl fixture under testdata/conformance/hurl
// (tests_ok, tests_ok_not_linted), a := RenderCurl(entry), then
// Import(a) -> b := RenderCurl(imported entry) must reproduce a exactly
// (a fixed point of export∘import), unless the entry uses a feature curl
// (or this package) cannot express at all — see excludedOptionNames and
// exclusionReason. Every entry is rendered in isolation (its own
// FromEntry/ToEntry) rather than sequentially with the rest of its file,
// so a variable only an earlier entry's capture or [Options] variable:
// would define is undefined on both the "a" and "b" side alike: the
// {{name}} literal it renders as either way is exactly the case the
// engine's own RenderCurl doc comment calls "fine".
func TestRoundtripConformance(t *testing.T) {
	var files []string
	for _, sub := range []string{"tests_ok", "tests_ok_not_linted"} {
		root := filepath.Join("..", "..", "..", "testdata", "conformance", "hurl", sub)
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".hurl") {
				return nil
			}
			files = append(files, path)
			return nil
		})
	}
	if len(files) == 0 {
		t.Fatal("no conformance fixtures found")
	}
	sort.Strings(files)

	total, covered := 0, 0
	excluded := map[string]int{}
	unparseableFiles := 0

	for _, path := range files {
		data, err := os.ReadFile(path) //nolint:gosec // G304: fixed test fixture path
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		f, err := syntax.Parse(path, data, syntax.DialectHurl)
		if err != nil {
			unparseableFiles++
			continue
		}
		// Absolute: RenderCurl's u.resolvedPath returns an absolute name
		// unchanged regardless of FileRoot (see engine/curl.go), and
		// sandbox.Root.ReadFile accepts an absolute path that is inside
		// its root. Reusing dir as FileRoot for both the original and
		// the reimported entry then genuinely round-trips a file
		// reference (an `output` option, a file body, a multipart file)
		// instead of resolvedPath joining an already-resolved relative
		// path a second time.
		dir, err := filepath.Abs(filepath.Dir(path))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}

		for i := 1; i <= len(f.Entries); i++ {
			total++
			entry := f.Entries[i-1]

			aEntries, errA := engine.NewRunner(engine.Options{FileRoot: dir, FromEntry: i, ToEntry: i}).RenderCurl(context.Background(), path, data)
			if failOrExcluded(t, path, i, entry, "", excluded, errA) {
				continue
			}
			if len(aEntries) == 0 {
				excluded["entry sends no request (skip: true, or repeat: 0)"]++
				continue
			}
			if failOrExcluded(t, path, i, entry, "", excluded, aEntries[0].Err) {
				continue
			}
			a := aEntries[0].Command

			res, errI := Import([]byte(a), syntax.DialectHurl)
			if errI != nil {
				if reason := exclusionReason(entry, a); reason != "" {
					excluded[reason]++
				} else {
					t.Errorf("%s entry %d: Import(%q) failed: %v", path, i, a, errI)
				}
				continue
			}
			if len(res.Skipped) > 0 {
				if reason := exclusionReason(entry, a); reason != "" {
					excluded[reason]++
				} else {
					t.Errorf("%s entry %d: reimporting %q was skipped: %v", path, i, a, res.Skipped)
				}
				continue
			}

			bEntries, errB := engine.NewRunner(engine.Options{FileRoot: dir, FromEntry: 1, ToEntry: 1}).RenderCurl(context.Background(), "reimported.hurl", syntax.Print(res.File))
			if failOrExcluded(t, path, i, entry, a, excluded, errB) {
				continue
			}
			if len(bEntries) == 0 {
				t.Errorf("%s entry %d: reimported %q rendered no command", path, i, a)
				continue
			}
			if failOrExcluded(t, path, i, entry, a, excluded, bEntries[0].Err) {
				continue
			}
			b := bEntries[0].Command

			if a == b {
				covered++
				continue
			}
			if reason := exclusionReason(entry, a); reason != "" {
				excluded[reason]++
				continue
			}
			t.Errorf("%s entry %d: export∘import is not a fixed point:\n  a = %s\n  b = %s", path, i, a, b)
		}
	}

	t.Logf("roundtrip conformance: covered %d / %d entries (%d files skipped: not curl syntax)", covered, total, unparseableFiles)
	if len(excluded) > 0 {
		reasons := make([]string, 0, len(excluded))
		for r := range excluded {
			reasons = append(reasons, r)
		}
		sort.Strings(reasons)
		for _, r := range reasons {
			t.Logf("  excluded %3d: %s", excluded[r], r)
		}
	}
}

// failOrExcluded reports a non-nil RenderCurl error as a test failure
// unless exclusionReason explains it (counted in excluded instead); it
// returns whether the caller should stop processing this entry (true for
// both outcomes of a non-nil err, false for a nil one).
func failOrExcluded(t *testing.T, path string, i int, entry *syntax.Entry, cmdSoFar string, excluded map[string]int, err error) bool {
	t.Helper()
	if err == nil {
		return false
	}
	reason := exclusionReason(entry, cmdSoFar)
	if reason == "" {
		reason = renderErrorReason(err)
	}
	if reason == "" {
		t.Errorf("%s entry %d: RenderCurl: %v", path, i, err)
		return true
	}
	excluded[reason]++
	return true
}

// renderErrorReason classifies a RenderCurl error this test's own setup
// causes, never this package's import/export code:
//
//   - An undefined variable substituted into a value with its own strict
//     syntax ("variables", one level more specific than the general
//     "stays a literal {{name}}, fine" case): an [Options] header: {{var}}
//     expecting "name:value", a bare JSON {{var}} standing in for a
//     number (internal/convert/curl/roundtrip_conformance_test.go's own
//     entries pass no variables at all, by design — an isolated,
//     server-less render), or a FileRef/multipart file whose own path is
//     an undefined {{var}} (post_file.hurl entry 2: `file,{{filename}};`)
//     — there is no real file to read at all, so it fails as a read
//     error rather than a parse error.
//   - A body file this test's FileRoot genuinely cannot read, confirmed
//     individually for every case this corpus actually has, all in
//     testdata/conformance/hurl/tests_ok/{fileroot,post}/: a ~15 MB
//     payload (post_large.bin) never checked into this trimmed fixture
//     subset; fileroot.hurl entry 2 reads fileroot.bin, which only exists
//     once entry 1's `output: fileroot.bin` option actually runs — this
//     test never sends anything; fileroot.hurl entry 3 reads
//     ../build/fileroot.bin, which its own comment says is fine only
//     "if descendant of the file root" — true for whatever --file-root
//     this fixture's original test suite runs it with, not for "the
//     fixture's own directory" (this test's default, and RenderCurl's
//     own default with no --file-root given): the sandbox is correctly
//     denying an escape from the root this test actually gives it, not
//     misbehaving.
func renderErrorReason(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "invalid header option value"), strings.Contains(msg, "Invalid JSON"), strings.Contains(msg, "file {{"):
		return "a value with its own strict syntax (an [Options] header: {{var}}, a bare JSON {{var}}, a FileRef/multipart path) can't keep {{name}} literal for an undefined variable without breaking that syntax or reading a real file"
	case strings.Contains(msg, "File read access"), strings.Contains(msg, "Unauthorized file access"):
		return "entry reads a local file this test's FileRoot cannot supply: missing from this trimmed fixture subset, written only once an earlier entry's output: option actually runs (never, here), or outside a --file-root this test does not reproduce (verified individually per entry, see comment on renderErrorReason)"
	}
	return ""
}

// exclusionReason reports why entry is allowed to fail the roundtrip
// fixed-point check, or "" if it should not.
func exclusionReason(entry *syntax.Entry, cmd string) string {
	for _, s := range entry.Request.Sections {
		switch s.Kind {
		case syntax.SectionOptions:
			for _, o := range s.Options {
				if reason, ok := excludedOptionNames[o.Name]; ok {
					return reason
				}
			}
		case syntax.SectionFormParams:
			// curl's own exporter omits an explicit Content-Type header
			// for a [FormParams]/[Form] section (spec.Form's implicit
			// type covers it, matching curl's own automatic behavior for
			// -d) and renders one --data flag per field; reimporting
			// necessarily produces a single already-'&'-joined literal
			// body with an explicit header (curl -d syntax cannot
			// distinguish "N fields" from "one already-joined string",
			// and a raw body has no implicit content type of its own).
			// TestRoundtrip's "post-form" case proves this still sends
			// the same bytes on the wire.
			return "[FormParams]/[Form]: curl renders it as separate --data flags with no explicit Content-Type; reimport must join and add one (same wire bytes)"
		}
	}
	if hasCookieStorageCommand(entry) {
		return "cookie-store comment commands (@cookie_storage_set/@cookie_storage_clear) have no curl equivalent"
	}
	if reason := binaryBodyReason(entry); reason != "" {
		return reason
	}
	if strings.Contains(cmd, "'Content-Type:'") {
		return "curl's own exporter hardcodes 'Content-Type:' (colon) to suppress an implicit type; reimporting it as an explicit empty header re-renders as 'Content-Type;' (semicolon) — same empty header either way"
	}
	return ""
}

// binaryBodyReason reports the exclusion for a *syntax.Base64/*syntax.Hex
// body: engine/curl.go's curlCommand renders it with a $'\xHH...' escape
// per byte; once this package's tokenizer decodes that back to raw bytes,
// a piece that happens to be valid UTF-8 text is indistinguishable from a
// literal text body (this package never invents a base64/hex body from
// curl's -d, which has no syntax for one) — same bytes, different
// encoding of the request file itself, not the wire.
func binaryBodyReason(entry *syntax.Entry) string {
	if entry.Request.Body == nil {
		return ""
	}
	switch entry.Request.Body.Value.(type) {
	case *syntax.Base64, *syntax.Hex:
		return "a base64/hex body, once curl-escaped and decoded back, is indistinguishable from a literal text body if its bytes are valid UTF-8 (same bytes, different body encoding)"
	}
	return ""
}

// hasCookieStorageCommand reports whether entry's leading comments carry a
// `# @cookie_storage_set:`/`# @cookie_storage_clear` command (mirrors
// engine's own unexported cookieCommands, which a test in another package
// cannot call directly).
func hasCookieStorageCommand(entry *syntax.Entry) bool {
	for _, lt := range entry.Request.LineTerminators {
		if lt.Comment == nil {
			continue
		}
		c := lt.Comment.Value
		if strings.Contains(c, "@cookie_storage_set:") || strings.Contains(c, "@cookie_storage_clear") {
			return true
		}
	}
	return false
}
