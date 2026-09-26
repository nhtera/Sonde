// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package opencollection

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// statusExpressions are the "expression" spellings this importer
// recognizes as referring to the response status, across the sources
// consulted (docs/decisions/0002-opencollection-mapping.md, "Scripts,
// tests and assertions").
var statusExpressions = map[string]bool{
	"res.status": true, "response.status": true, "res.statuscode": true, "response.statuscode": true,
}

// statusOperators are the "operator" spellings treated as equality.
var statusOperators = map[string]bool{"eq": true, "equals": true, "==": true}

// buildRuntime turns rt's scripts, assertions and actions into comments
// (never executed) plus, for at most one trivially safe status assertion,
// the entry's expected response (mapping doc, "Scripts, tests and
// assertions — never executed").
func buildRuntime(name string, rt runtimeBlock) (comments []string, resp *syntax.ResponseSpec, warns []convert.Warning) {
	for _, s := range rt.Scripts {
		if s.Code == "" {
			continue
		}
		comments = append(comments, "opencollection "+s.Type+" script:\n"+s.Code)
		warns = append(warns, convert.Warning{Kind: convert.WarnScript,
			Message: fmt.Sprintf("%s: %s script kept as a comment, never executed", name, s.Type)})
	}

	statusUsed := false
	for _, a := range rt.Assertions {
		if a.Disabled {
			continue
		}
		if !statusUsed {
			if n, ok := statusAssertionValue(a); ok {
				resp = &syntax.ResponseSpec{Status: strconv.Itoa(n)}
				statusUsed = true
				continue
			}
		}
		comments = append(comments, fmt.Sprintf("opencollection assertion: %s %s %s", a.Expression, a.Operator, a.Value))
		warns = append(warns, convert.Warning{Kind: convert.WarnScript,
			Message: fmt.Sprintf("%s: assertion %q kept as a comment, never executed", name, a.Expression)})
	}

	for _, act := range rt.Actions {
		comments = append(comments, fmt.Sprintf("opencollection %s action (phase: %s)", act.Type, act.Phase))
		warns = append(warns, convert.Warning{Kind: convert.WarnScript,
			Message: fmt.Sprintf("%s: %s action kept as a comment, never executed", name, act.Type)})
	}
	return comments, resp, warns
}

// statusAssertionValue reports the status code a trivially translates to,
// if it is an equality check against a recognized status expression with
// an in-range value.
func statusAssertionValue(a assertion) (int, bool) {
	if !statusExpressions[strings.ToLower(strings.TrimSpace(a.Expression))] || !statusOperators[strings.ToLower(strings.TrimSpace(a.Operator))] {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(a.Value))
	if err != nil || n < 100 || n > 599 {
		return 0, false
	}
	return n, true
}
