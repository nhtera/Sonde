// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/value"
)

// Regex returns the regex of a filter, query or predicate argument: a
// /regex/ literal, or a string template compiled after rendering. span
// locates errors on a literal.
func (e *Env) Regex(n syntax.Node, span syntax.Span) (value.Regex, error) {
	switch n := n.(type) {
	case *syntax.Regex:
		re, err := value.NewRegex(n.Pattern)
		if err != nil {
			return value.Regex{}, runerr.New(span, runerr.InvalidRegex, false)
		}
		return re, nil
	case *syntax.Template:
		s, err := e.Render(n)
		if err != nil {
			return value.Regex{}, err
		}
		re, err := value.NewRegex(s)
		if err != nil {
			return value.Regex{}, runerr.New(n.Span, runerr.InvalidRegex, false)
		}
		return re, nil
	}
	return value.Regex{}, runerr.New(span, runerr.InvalidRegex, false)
}
