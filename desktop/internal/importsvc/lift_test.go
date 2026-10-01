// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package importsvc

import (
	"testing"

	"github.com/nhtera/sonde/internal/syntax"
)

// TestCandidatesBasicAuthScheme: Basic auth scheme becomes basic_auth, not token.
func TestCandidatesBasicAuthScheme(t *testing.T) {
	src := []byte("GET https://api.test/data\nAuthorization: Basic dXNlcjpwYXNz\n")
	cands, _, err := candidates(src, syntax.DialectHurl, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range cands {
		if c.Name == "basic_auth" && c.Where == "Authorization header" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no basic_auth candidate found in %v", cands)
	}
}

// TestCandidatesPlaceholderInValueSkipped: literal function skips values with placeholders.
func TestCandidatesPlaceholderInValueSkipped(t *testing.T) {
	// A value with a placeholder like "Bearer secret{{suffix}}" should be skipped
	src := []byte("GET https://api.test/data\nAuthorization: Bearer secret{{suffix}}\n")
	cands, _, err := candidates(src, syntax.DialectHurl, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	// Should have no token candidate because the value contains {{
	for _, c := range cands {
		if c.Name == "token" && c.Where == "Authorization header" {
			t.Errorf("value with placeholder should be skipped, but found: %+v", c)
		}
	}
}

// TestCandidatesPlaceholderValuesSkipped: values with {{placeholder}} are skipped.
func TestCandidatesPlaceholderValuesSkipped(t *testing.T) {
	src := []byte("GET https://api.test/data\nAuthorization: Bearer {{existing_token}}\nX-Api-Key: {{api_key}}\n")
	cands, _, err := candidates(src, syntax.DialectHurl, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	// Should have no candidates because values contain placeholders
	if len(cands) > 0 {
		t.Errorf("placeholders should be skipped, but found %d candidates: %+v", len(cands), cands)
	}
}

// TestCandidatesQueryKeysWithoutValues: query keys without values are skipped.
func TestCandidatesQueryKeysWithoutValues(t *testing.T) {
	src := []byte("GET https://api.test/data?api_key&other=value\n")
	cands, _, err := candidates(src, syntax.DialectHurl, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	// api_key has no value, other is not a credential: nothing to lift.
	if len(cands) != 0 {
		t.Errorf("candidates %+v", cands)
	}
}

// TestCandidatesNamesDedupedAgainstTaken: names are deduped against taken and each other.
func TestCandidatesNamesDedupedAgainstTaken(t *testing.T) {
	src := []byte("GET https://api.test/data?api_key=secret1&api_key=secret2\nX-Api-Key: secret3\n")
	taken := map[string]bool{"api_key": true} // api_key already in environment
	cands, _, err := candidates(src, syntax.DialectHurl, taken)
	if err != nil {
		t.Fatal(err)
	}
	// Collect candidate names
	names := make(map[string]int)
	for _, c := range cands {
		names[c.Name]++
	}
	// Should have api_key_2, api_key_3 or similar (not duplicates)
	for name, count := range names {
		if count > 1 {
			t.Errorf("duplicate name %q appears %d times", name, count)
		}
	}
	// Should not use "api_key" since it's taken
	if names["api_key"] > 0 {
		t.Error("taken name api_key should not be used")
	}
}

// TestLiftValidatesEntryCount: lift validates that the result still has the same number of entries.
func TestLiftValidatesEntryCount(t *testing.T) {
	src := []byte("GET https://api.test/a\nHTTP 200\n\nPOST https://api.test/b\nAuthorization: Bearer secret-token\nHTTP 201\n")
	cands, _, err := candidates(src, syntax.DialectHurl, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) == 0 {
		t.Fatal("no candidates found")
	}
	// Lift should work normally and preserve entry count
	out, values, err := lift(src, syntax.DialectHurl, cands, []int{cands[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	// Verify that entries are preserved
	before, _ := syntax.Parse("test.hurl", src, syntax.DialectHurl)
	after, _ := syntax.Parse("test.hurl", out, syntax.DialectHurl)
	if len(before.Entries) != len(after.Entries) {
		t.Errorf("entry count changed: %d -> %d", len(before.Entries), len(after.Entries))
	}
	if len(values) == 0 {
		t.Error("no values were lifted")
	}
}

// TestLiftBasicAuthValue: Basic auth in Authorization header extracts only the value after the scheme.
func TestLiftBasicAuthValue(t *testing.T) {
	src := []byte("GET https://api.test/data\nAuthorization: Basic dXNlcjpwYXNz\n")
	cands, _, err := candidates(src, syntax.DialectHurl, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	out, values, err := lift(src, syntax.DialectHurl, cands, []int{cands[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if values["basic_auth"] != "dXNlcjpwYXNz" {
		t.Errorf("basic_auth value %q, want dXNlcjpwYXNz", values["basic_auth"])
	}
	if !contains(out, "Bearer {{basic_auth}}") && !contains(out, "Basic {{basic_auth}}") {
		t.Errorf("lifted value not in output: %s", out)
	}
}

// TestLiftMultipleCandidatesSelectSome: lifting selects only picked IDs.
func TestLiftMultipleCandidatesSelectSome(t *testing.T) {
	src := []byte("GET https://api.test/a?api_key=key1\nAuthorization: Bearer token1\nX-Api-Key: key2\n")
	cands, _, err := candidates(src, syntax.DialectHurl, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) < 2 {
		t.Fatalf("expected at least 2 candidates, got %d", len(cands))
	}
	// Lift only the first one
	_, values, err := lift(src, syntax.DialectHurl, cands, []int{cands[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	// Should have only one value lifted
	if len(values) != 1 {
		t.Errorf("lifted %d values, want 1: %+v", len(values), values)
	}
}

// TestLiftPreservesEntryStructure: lift doesn't change the number of entries.
func TestLiftPreservesEntryStructure(t *testing.T) {
	src := []byte("GET https://api.test/a\nHTTP 200\n\nPOST https://api.test/b\nAuthorization: Bearer secret\nHTTP 201\n")
	before, _ := syntax.Parse("test.hurl", src, syntax.DialectHurl)
	entryCountBefore := len(before.Entries)

	cands, _, err := candidates(src, syntax.DialectHurl, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := lift(src, syntax.DialectHurl, cands, []int{cands[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := syntax.Parse("test.hurl", out, syntax.DialectHurl)
	entryCountAfter := len(after.Entries)

	if entryCountBefore != entryCountAfter {
		t.Errorf("entry count changed: %d -> %d", entryCountBefore, entryCountAfter)
	}
}

func contains(b []byte, s string) bool {
	for i := 0; i <= len(b)-len(s); i++ {
		if string(b[i:i+len(s)]) == s {
			return true
		}
	}
	return false
}

// TestCandidatesEscapedReported: a credential written with escapes is not
// offered (its span is not its value) but named, so the user is told.
func TestCandidatesEscapedReported(t *testing.T) {
	src := []byte("GET https://api.test/data\nX-Api-Key: abc\\#def\nAuthorization: Bearer {{t}}\n")
	cands, escaped, err := candidates(src, syntax.DialectHurl, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 0 || len(escaped) != 1 || escaped[0] != "X-Api-Key header" {
		t.Errorf("candidates %+v, escaped %v", cands, escaped)
	}
}
