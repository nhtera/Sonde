// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
)

// variants returns the deduplicated encoded forms of value that Redact also
// masks: base64 (standard and URL-safe, padded and unpadded), URL-escaped
// (query and path-segment rules, plus curl's own stricter percent-encoding
// — see curlURLEscaped), JSON string content (with and without HTML
// escaping), and curl's own $'...' backslash escaping (see ansiCEscaped),
// also of the JSON string content.
// The raw value itself, and any variant identical to it or empty, are
// omitted.
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
	add(curlURLEscaped(value))
	add(ansiCEscaped(value))
	// A secret inside a JSON string of a body curl quotes with $'...'.
	add(ansiCEscaped(jsonEscaped(value, false)))
	add(ansiCEscaped(jsonEscaped(value, true)))

	return out
}

// curlURLEscaped mirrors engine/curl.go's escapeURL, the encoding a curl
// command line's query and form-urlencoded values are always rendered
// with: every byte outside ASCII letters and digits becomes %XX (upper
// case hex) — stricter than url.QueryEscape/PathEscape, which keep
// "-_.~" (and, for QueryEscape, " " as "+") unescaped, so a secret with
// one of those characters (routine in an API key, e.g. "sk_live_...")
// would otherwise survive that encoding unmasked. redact cannot import
// engine (a leaf package importing the layer above it), so this mirrors
// the algorithm rather than calling it; the two must be kept in sync by
// hand if either changes.
func curlURLEscaped(value string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0xF])
	}
	return b.String()
}

// ansiCEscaped mirrors the inner text of engine/curl.go's shellString
// once it switches to its $'...' form (triggered by a newline, tab or
// single quote anywhere in the value — after which curl.go escapes every
// backslash too, not just the one it just added): a backslash, a single
// quote, a newline or a tab each become a two-character backslash escape
// (\\, \', \n, \t). A value with none of those four characters renders in
// curl.go's plain '...' form instead, identical to the raw value, so this
// variant is only ever new information when at least one of them is
// present — exactly the case curl.go itself switches encodings for.
func ansiCEscaped(value string) string {
	r := strings.NewReplacer("\n", `\n`, "\t", `\t`, "'", `\'`, `\`, `\\`)
	return r.Replace(value)
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
