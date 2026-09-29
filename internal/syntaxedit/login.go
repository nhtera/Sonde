// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntaxedit

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nhtera/sonde/internal/syntax"
)

// LoginSpec is an entry that logs in and captures a token from its JSON
// response, for the entries after it (a form's "add login" action, an
// OAuth2 import suggestion).
type LoginSpec struct {
	// Request is the login request; its Response status defaults to 200.
	Request syntax.EntrySpec
	// Capture is the variable the token is captured in, from the response
	// at JSONPath (e.g. "$.access_token").
	Capture  string
	JSONPath string
	// Redact makes the capture a secret.
	Redact bool
}

var identRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// AddLoginEntry inserts the login entry spec before entry before
// (1-based; past the last entry: at the end).
func AddLoginEntry(name string, src []byte, before int, spec LoginSpec) (*Result, error) {
	if !identRE.MatchString(spec.Capture) {
		return nil, fmt.Errorf("%w: capture name %q", ErrInvalid, spec.Capture)
	}
	if err := singleLine("the JSONPath", spec.JSONPath); err != nil {
		return nil, err
	}
	d, err := parse(name, src)
	if err != nil {
		return nil, err
	}
	req := spec.Request
	if req.Response == nil {
		req.Response = &syntax.ResponseSpec{Status: "200"}
	}
	text, err := buildEntries(name, []syntax.EntrySpec{req})
	if err != nil {
		return nil, err
	}
	capture := spec.Capture + ": jsonpath " + quote(spec.JSONPath)
	if spec.Redact {
		capture += " redact"
	}
	text += "[Captures]\n" + capture + "\n"

	var at int
	var lead, trail string
	if before >= 1 && before <= len(d.entries) {
		at, trail = d.firstLineOf(d.entries[before-1]), "\n"
	} else {
		at, lead = d.insertionPoint(len(src))
		if len(src) > 0 {
			lead += "\n"
		}
	}
	index := min(max(before, 1), len(d.entries)+1)
	want := len(d.entries) + 1
	return d.splice(at, at, lead+text+trail, func(nd *doc) error {
		if len(nd.entries) != want {
			return fmt.Errorf("%w: %d entries, not %d", ErrInvalid, len(nd.entries), want)
		}
		caps := nd.entries[index-1].sections[Captures]
		if caps == nil || len(caps.rows) != 1 || caps.rows[0].Key != spec.Capture {
			return fmt.Errorf("%w: the capture does not read back", ErrInvalid)
		}
		return nil
	})
}

// quote writes s as a double-quoted string.
func quote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}
