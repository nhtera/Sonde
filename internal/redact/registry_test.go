// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"encoding/base64"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestAddEmptyValueIgnored(t *testing.T) {
	r := New()
	if short := r.Add("token", ""); short {
		t.Errorf(`Add("token", "") reported short`)
	}
	if r.Len() != 0 {
		t.Errorf("Len() = %d, want 0", r.Len())
	}
	if got := r.Redact("anything"); got != "anything" {
		t.Errorf("Redact after empty Add = %q", got)
	}
}

func TestAddShortFlag(t *testing.T) {
	r := New()
	tests := []struct {
		value string
		want  bool
	}{
		{"a", true},      // 1 rune < MinLength
		{"abc", true},    // 3 runes < MinLength
		{"abcd", false},  // 4 runes == MinLength
		{"abcde", false}, // 5 runes
		{"日本", true},     // 2 runes, unicode
	}
	for _, tt := range tests {
		if got := r.Add("name", tt.value); got != tt.want {
			t.Errorf("Add(name, %q) short = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestAddReaddKnownValueReturnsFalse(t *testing.T) {
	r := New()
	if short := r.Add("token", "ab"); !short {
		t.Fatalf("first Add should report short")
	}
	if short := r.Add("token", "ab"); short {
		t.Errorf("re-adding a known value under the same name reported short")
	}
	if short := r.Add("other-name", "ab"); short {
		t.Errorf("re-adding a known value under a different name reported short")
	}
	if r.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (value counted once)", r.Len())
	}
}

func TestAddAppendOnlyAcrossName(t *testing.T) {
	r := New()
	r.Add("token", "first-value")
	r.Add("token", "second-value")
	got := r.Redact("first-value then second-value")
	if want := "*** then ***"; got != want {
		t.Errorf("Redact = %q, want %q (both values still masked)", got, want)
	}
}

func TestRedactVariants(t *testing.T) {
	secret := "s3cr3t p@ss/word"
	tests := []struct {
		name    string
		variant string
	}{
		{"raw", secret},
		{"base64-std", base64.StdEncoding.EncodeToString([]byte(secret))},
		{"base64-std-raw", base64.RawStdEncoding.EncodeToString([]byte(secret))},
		{"base64-url", base64.URLEncoding.EncodeToString([]byte(secret))},
		{"base64-url-raw", base64.RawURLEncoding.EncodeToString([]byte(secret))},
		{"query-escape", url.QueryEscape(secret)},
		{"path-escape", url.PathEscape(secret)},
		{"json-plain", jsonEscaped(secret, false)},
		{"json-html", jsonEscaped(secret, true)},
	}

	r := New()
	r.Add("secret", secret)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := "prefix " + tt.variant + " suffix"
			want := "prefix " + Mask + " suffix"
			if got := r.Redact(in); got != want {
				t.Errorf("Redact(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

func TestRedactURLEscapedSpacesAndAmpersand(t *testing.T) {
	secret := "a b&c"
	r := New()
	r.Add("secret", secret)
	in := "q=" + url.QueryEscape(secret) + "&x=1"
	want := "q=" + Mask + "&x=1"
	if got := r.Redact(in); got != want {
		t.Errorf("Redact(%q) = %q, want %q", in, got, want)
	}
}

func TestRedactJSONEscapedQuotesBackslashNewlineAngle(t *testing.T) {
	secret := "a\"b\\c\nd<e" //nolint:gosec // G101: test fixture, not a real credential
	r := New()
	r.Add("secret", secret)

	plain := jsonEscaped(secret, false)
	html := jsonEscaped(secret, true)
	if plain == html {
		t.Fatalf("expected plain/html JSON escapes to differ for %q", secret)
	}
	for _, variant := range []string{plain, html} {
		in := `{"field":"` + variant + `"}`
		want := `{"field":"` + Mask + `"}`
		if got := r.Redact(in); got != want {
			t.Errorf("Redact(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRedactLongestFirstOverlap(t *testing.T) {
	r := New()
	r.Add("short", "abc")
	r.Add("long", "abcdef")
	if got, want := r.Redact("xabcdefx"), "x***x"; got != want {
		t.Errorf("Redact = %q, want %q", got, want)
	}
}

func TestRedactUnicodeSecret(t *testing.T) {
	r := New()
	secret := "sécrét-🔑" //nolint:gosec // G101: test fixture, not a real credential
	r.Add("secret", secret)
	if got, want := r.Redact("before "+secret+" after"), "before *** after"; got != want {
		t.Errorf("Redact = %q, want %q", got, want)
	}
}

func TestRedactSpecialRegexChars(t *testing.T) {
	r := New()
	secret := `a.b*c(d)[e]+f?` //nolint:gosec // G101: test fixture, not a real credential
	r.Add("secret", secret)
	if got, want := r.Redact("x"+secret+"y"), "x***y"; got != want {
		t.Errorf("Redact = %q, want %q", got, want)
	}
	// A similar-looking but non-matching string must survive untouched,
	// confirming this is a literal match rather than a regex.
	other := "aXbXcXdXeXfX"
	if got := r.Redact(other); got != other {
		t.Errorf("Redact(%q) = %q, want unchanged", other, got)
	}
}

func TestRedactEmptyRegistryNoAllocation(t *testing.T) {
	r := New()
	s := "nothing secret here"
	allocs := testing.AllocsPerRun(100, func() {
		_ = r.Redact(s)
	})
	if allocs != 0 {
		t.Errorf("Redact on empty registry allocated %.0f times, want 0", allocs)
	}
	if got := r.Redact(s); got != s {
		t.Errorf("Redact = %q, want unchanged", got)
	}
}

func TestRedactBytes(t *testing.T) {
	r := New()
	r.Add("secret", "hunter2")
	if got, want := string(r.RedactBytes([]byte("password=hunter2"))), "password=***"; got != want {
		t.Errorf("RedactBytes = %q, want %q", got, want)
	}

	empty := New()
	if got, want := string(empty.RedactBytes([]byte("unchanged"))), "unchanged"; got != want {
		t.Errorf("RedactBytes on empty registry = %q, want %q", got, want)
	}
}

func TestValuesSortedSnapshot(t *testing.T) {
	r := New()
	r.Add("a", "zeta")
	r.Add("b", "alpha")
	r.Add("c", "mu")

	got := r.Values()
	want := []string{"alpha", "mu", "zeta"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Values() = %v, want %v", got, want)
	}

	// The returned slice is a copy: mutating it must not affect the registry.
	got[0] = "tampered"
	if got2 := r.Values(); got2[0] == "tampered" {
		t.Errorf("Values() returned a slice shared with the registry")
	}
}

func TestLen(t *testing.T) {
	r := New()
	if r.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", r.Len())
	}
	r.Add("a", "one")
	r.Add("b", "two")
	r.Add("a", "one") // duplicate value
	if r.Len() != 2 {
		t.Errorf("Len() = %d, want 2", r.Len())
	}
}

func TestRedactDeterministic(t *testing.T) {
	r1 := New()
	r1.Add("a", "alpha")
	r1.Add("b", "beta")

	r2 := New()
	r2.Add("b", "beta")
	r2.Add("a", "alpha")

	in := "alpha and beta together"
	if got1, got2 := r1.Redact(in), r2.Redact(in); got1 != got2 {
		t.Errorf("Redact not deterministic across insertion order: %q vs %q", got1, got2)
	}
}

func TestConcurrentAddAndRedact(t *testing.T) {
	r := New()
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		i := i
		wg.Add(2)
		go func() {
			defer wg.Done()
			r.Add("worker", strings.Repeat(string(rune('a'+i%26)), 3)+"-secret")
		}()
		go func() {
			defer wg.Done()
			_ = r.Redact("some text with a-secret possibly inside it")
		}()
	}
	wg.Wait()
	if r.Len() == 0 {
		t.Fatalf("expected values to be registered")
	}
}

// Overlapping secrets must not leave part of either visible.
func TestRedactOverlappingSecrets(t *testing.T) {
	r := New()
	r.Add("a", "xxs3cr")
	r.Add("b", "s3cr3tTOKEN")
	tests := map[string]string{
		"id=xxs3cr3tTOKEN":   "id=***",
		"xxs3cr s3cr3tTOKEN": "*** ***",
		"xxs3crs3cr3tTOKEN!": "***!",
		"nothing":            "nothing",
	}
	for in, want := range tests {
		if got := r.Redact(in); got != want {
			t.Errorf("Redact(%q) = %q, want %q", in, got, want)
		}
	}
	r2 := New()
	r2.Add("x", "aa")
	if got := r2.Redact("aaaaa"); got != "***" {
		t.Errorf("repeated overlapping = %q", got)
	}
}
