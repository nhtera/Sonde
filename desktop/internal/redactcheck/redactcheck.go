// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package redactcheck is the sentinel harness of the tests: it fails a
// test when a secret value, or a form of it the redactor also masks,
// appears in anything the app hands to the frontend.
package redactcheck

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// Forms returns secret and the encodings of it that must not appear
// either: base64 (standard and URL-safe, padded or not), URL escaping and
// JSON escaping.
func Forms(secret string) []string {
	j, _ := json.Marshal(secret)
	forms := []string{
		secret,
		base64.StdEncoding.EncodeToString([]byte(secret)),
		base64.RawStdEncoding.EncodeToString([]byte(secret)),
		base64.URLEncoding.EncodeToString([]byte(secret)),
		base64.RawURLEncoding.EncodeToString([]byte(secret)),
		url.QueryEscape(secret),
		url.PathEscape(secret),
		strings.Trim(string(j), `"`),
	}
	seen := map[string]bool{}
	out := forms[:0]
	for _, f := range forms {
		if f != "" && !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

// Find returns a description of the first form of a secret found in data,
// or "".
func Find(data []byte, secrets ...string) string {
	s := string(data)
	for _, secret := range secrets {
		for _, f := range Forms(secret) {
			if i := strings.Index(s, f); i >= 0 {
				return fmt.Sprintf("secret %q (as %q) at byte %d", mask(secret), mask(f), i)
			}
		}
	}
	return ""
}

// AssertNoSecret fails t when v, marshaled as the frontend receives it
// (JSON), holds a secret. what names v in the failure.
func AssertNoSecret(t testing.TB, what string, v any, secrets ...string) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	if found := Find(data, secrets...); found != "" {
		t.Errorf("%s leaks %s", what, found)
	}
}

// AssertNoSecretBytes fails t when raw bytes (a body) hold a secret.
func AssertNoSecretBytes(t testing.TB, what string, data []byte, secrets ...string) {
	t.Helper()
	if found := Find(data, secrets...); found != "" {
		t.Errorf("%s leaks %s", what, found)
	}
}

// mask keeps a secret's shape in a failure message without printing it.
func mask(s string) string {
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	return s[:2] + strings.Repeat("*", len(s)-4) + s[len(s)-2:]
}
