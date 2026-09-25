// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"encoding/base64"
	"net/url"
	"sort"
	"testing"
)

func TestVariantsIncludesExpectedEncodings(t *testing.T) {
	value := "sé \"cret\"&<x>"
	got := variants(value)

	have := make(map[string]bool, len(got))
	for _, v := range got {
		have[v] = true
	}

	want := []string{
		base64.StdEncoding.EncodeToString([]byte(value)),
		base64.RawStdEncoding.EncodeToString([]byte(value)),
		base64.URLEncoding.EncodeToString([]byte(value)),
		base64.RawURLEncoding.EncodeToString([]byte(value)),
		url.QueryEscape(value),
		url.PathEscape(value),
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("variants(%q) missing %q", value, w)
		}
	}

	// '<', '>' and '&' make the HTML-escaped and plain JSON forms differ.
	plain := jsonEscaped(value, false)
	html := jsonEscaped(value, true)
	if plain == html {
		t.Fatalf("expected plain/html JSON variants to differ for %q", value)
	}
	if !have[plain] {
		t.Errorf("variants(%q) missing plain JSON variant %q", value, plain)
	}
	if !have[html] {
		t.Errorf("variants(%q) missing HTML-escaped JSON variant %q", value, html)
	}
}

func TestVariantsExcludesRawAndEmpty(t *testing.T) {
	// "abc" has no characters that base64, URL or JSON escaping would
	// change other than by encoding it outright, so its JSON-escaped form
	// equals the raw value and must be skipped.
	value := "abc"
	got := variants(value)
	for _, v := range got {
		if v == value {
			t.Errorf("variants(%q) contains the raw value", value)
		}
		if v == "" {
			t.Errorf("variants(%q) contains an empty variant", value)
		}
	}
}

func TestVariantsDeduplicated(t *testing.T) {
	// A value whose length is a multiple of 3 has identical padded and
	// unpadded standard base64 forms; they must not appear twice.
	value := "abcdef"
	got := variants(value)
	seen := make(map[string]struct{}, len(got))
	for _, v := range got {
		if _, ok := seen[v]; ok {
			t.Fatalf("variants(%q) contains duplicate %q", value, v)
		}
		seen[v] = struct{}{}
	}
}

func TestVariantsDeterministic(t *testing.T) {
	value := "some-secret-Value_123"
	a := variants(value)
	b := variants(value)
	sort.Strings(a)
	sort.Strings(b)
	if len(a) != len(b) {
		t.Fatalf("length mismatch: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("mismatch at %d: %q vs %q", i, a[i], b[i])
		}
	}
}
