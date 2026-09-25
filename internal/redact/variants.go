// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/url"
)

// variants returns the deduplicated encoded forms of value that Redact also
// masks: base64 (standard and URL-safe, padded and unpadded), URL-escaped
// (query and path-segment rules), and JSON string content (with and
// without HTML escaping). The raw value itself, and any variant identical
// to it or empty, are omitted.
func variants(value string) []string {
	seen := map[string]struct{}{value: {}}
	var out []string
	add := func(s string) {
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}

	b := []byte(value)
	add(base64.StdEncoding.EncodeToString(b))
	add(base64.RawStdEncoding.EncodeToString(b))
	add(base64.URLEncoding.EncodeToString(b))
	add(base64.RawURLEncoding.EncodeToString(b))
	add(url.QueryEscape(value))
	add(url.PathEscape(value))
	add(jsonEscaped(value, false))
	add(jsonEscaped(value, true))

	return out
}

// jsonEscaped returns what encoding/json would place between the quotes of
// a JSON string literal for value, with HTML escaping (<, >, &) enabled or
// disabled. It returns "" if value cannot be encoded, which cannot happen
// for a Go string.
func jsonEscaped(value string, escapeHTML bool) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(escapeHTML)
	if err := enc.Encode(value); err != nil {
		return ""
	}
	quoted := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	if len(quoted) < 2 {
		return ""
	}
	return string(quoted[1 : len(quoted)-1])
}
