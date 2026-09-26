// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package convert

import (
	"strings"

	"github.com/nhtera/sonde/internal/syntax"
)

// dynamicVariables maps the dynamic variables of other tools ({{$uuid}})
// to the Sonde function with the same meaning.
var dynamicVariables = map[string]string{
	"$uuid":         "newUuid",
	"$guid":         "newUuid",
	"$randomUUID":   "newUuid",
	"$random.uuid":  "newUuid",
	"$isoTimestamp": "newDate",
}

// functionNames are the Sonde template functions; a variable of another
// tool with one of these names is renamed.
var functionNames = map[string]bool{"newDate": true, "newUuid": true, "getEnv": true}

// VariableName returns name as a valid Sonde variable name: every
// character other than a letter, a digit, '_' or '-' becomes '_', and a
// name clashing with a Sonde function gets a "var_" prefix. Importers use
// it for both the placeholders and the variable definitions, so the two
// always match.
func VariableName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < 0x80 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	out := b.String()
	if out == "" {
		out = "_"
	}
	if functionNames[out] {
		out = "var_" + out
	}
	return out
}

// ParseText converts a string holding {{name}} placeholders, as Postman,
// Bruno and .http files write them, to a syntax.Text. Spaces inside the
// braces are ignored; a name is made valid with VariableName; a dynamic
// variable ({{$uuid}}) becomes the matching Sonde function, or, without
// one, a variable of the same name and a WarnDynamicVariable warning. An
// unclosed "{{" is literal text.
func ParseText(s string) (syntax.Text, []Warning) {
	var t syntax.Text
	var warns []Warning
	for {
		open := strings.Index(s, "{{")
		if open < 0 {
			break
		}
		end := strings.Index(s[open+2:], "}}")
		if end < 0 {
			break
		}
		name := strings.TrimSpace(s[open+2 : open+2+end])
		if strings.Contains(name, "{") {
			// "{{{x}}}": the first brace is text.
			t = append(t, syntax.Lit(s[:open+1]))
			s = s[open+1:]
			continue
		}
		if name == "" {
			t = append(t, syntax.Lit(s[:open+2+end+2]))
			s = s[open+2+end+2:]
			continue
		}
		if open > 0 {
			t = append(t, syntax.Lit(s[:open]))
		}
		s = s[open+2+end+2:]
		if strings.HasPrefix(name, "$") {
			if fn := dynamicVariables[name]; fn != "" {
				t = append(t, syntax.Var(fn))
				continue
			}
			warns = append(warns, Warning{Kind: WarnDynamicVariable,
				Message: "{{" + name + "}} has no Sonde equivalent; it is the variable " + VariableName(strings.TrimPrefix(name, "$")) + " to set"})
			name = strings.TrimPrefix(name, "$")
		}
		t = append(t, syntax.Var(VariableName(name)))
	}
	if s != "" {
		t = append(t, syntax.Lit(s))
	}
	return t, warns
}
