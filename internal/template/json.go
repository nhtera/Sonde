// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"strconv"
	"strings"

	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/syntax"
)

// RenderMultiline renders a multiline string. Raw strings are not
// templated; a GraphQL string becomes the JSON document
// `{"query":…,"variables":…}`.
func (e *Env) RenderMultiline(m *syntax.MultilineString) (string, error) {
	switch m.Kind {
	case syntax.MultilineRaw:
		return templateSource(m.Value), nil
	case syntax.MultilineGraphQL:
		query, err := e.Render(m.Value)
		if err != nil {
			return "", err
		}
		q := jsonString(strings.TrimSpace(query))
		if m.Variables == nil {
			return `{"query":` + q + `}`, nil
		}
		vars, err := e.RenderJSON(m.Variables.Value, false)
		if err != nil {
			return "", err
		}
		return `{"query":` + q + `,"variables":` + vars + `}`, nil
	}
	return e.Render(m.Value)
}

// RenderJSON renders a JSON value with its placeholders; keepSpace keeps
// the white space as written.
func (e *Env) RenderJSON(v syntax.JSONValue, keepSpace bool) (string, error) {
	var b strings.Builder
	if err := e.writeJSON(&b, v, keepSpace); err != nil {
		return "", err
	}
	return b.String(), nil
}

func (e *Env) writeJSON(b *strings.Builder, v syntax.JSONValue, keep bool) error {
	switch v := v.(type) {
	case *syntax.Null:
		b.WriteString("null")
	case *syntax.Boolean:
		b.WriteString(strconv.FormatBool(v.Value))
	case *syntax.JSONNumber:
		b.WriteString(v.Source)
	case *syntax.Template:
		b.WriteByte('"')
		for _, el := range v.Elements {
			switch el := el.(type) {
			case *syntax.TemplateString:
				b.WriteString(el.Source)
			case *syntax.Placeholder:
				s, err := e.RenderExpr(el.Expr)
				if err != nil {
					return err
				}
				b.WriteString(escapeJSONText(s))
			}
		}
		b.WriteByte('"')
	case *syntax.Placeholder:
		s, err := e.RenderExpr(v.Expr)
		if err != nil {
			return err
		}
		if !isJSONScalarPrefix(s) {
			err := runerr.New(v.Expr.Span, runerr.InvalidJSON, false)
			err.Value = s
			return err
		}
		b.WriteString(s)
	case *syntax.JSONList:
		b.WriteByte('[')
		if keep {
			b.WriteString(v.Space0)
		}
		for i, el := range v.Elements {
			if i > 0 {
				b.WriteByte(',')
			}
			if keep {
				b.WriteString(el.Space0)
			}
			if err := e.writeJSON(b, el.Value, keep); err != nil {
				return err
			}
			if keep {
				b.WriteString(el.Space1)
			}
		}
		b.WriteByte(']')
	case *syntax.JSONObject:
		b.WriteByte('{')
		if keep {
			b.WriteString(v.Space0)
		}
		for i, el := range v.Elements {
			if i > 0 {
				b.WriteByte(',')
			}
			if keep {
				name, err := e.Render(el.Name)
				if err != nil {
					return err
				}
				b.WriteString(el.Space0 + `"` + name + `"` + el.Space1 + ":" + el.Space2)
			} else {
				b.WriteString(`"` + templateDisplay(el.Name) + `":`)
			}
			if err := e.writeJSON(b, el.Value, keep); err != nil {
				return err
			}
			if keep {
				b.WriteString(el.Space3)
			}
		}
		b.WriteByte('}')
	}
	return nil
}

// isJSONScalarPrefix reports whether s starts like a JSON number, boolean
// or null, the only values a bare placeholder may produce.
func isJSONScalarPrefix(s string) bool {
	t := strings.TrimPrefix(s, "-")
	if t != "" && t[0] >= '0' && t[0] <= '9' {
		return true
	}
	return strings.HasPrefix(s, "true") || strings.HasPrefix(s, "false") || strings.HasPrefix(s, "null")
}

// escapeJSONText escapes the characters of s that would break a JSON string.
func escapeJSONText(s string) string {
	if !strings.ContainsAny(s, "\"\\\n\r\t") {
		return s
	}
	r := strings.NewReplacer(`"`, `\"`, `\`, `\\`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return r.Replace(s)
}

// jsonString encodes s as a JSON string literal; only quotes, backslashes
// and control characters are escaped.
func jsonString(s string) string {
	const hex = "0123456789abcdef"
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 0x20 {
				b.WriteString(`\u00`)
				b.WriteByte(hex[c>>4])
				b.WriteByte(hex[c&0xf])
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// templateSource returns a template as written, without delimiters.
func templateSource(t *syntax.Template) string {
	var b strings.Builder
	for _, el := range t.Elements {
		switch el := el.(type) {
		case *syntax.TemplateString:
			b.WriteString(el.Source)
		case *syntax.Placeholder:
			b.WriteString("{{" + el.Space0.Value + el.Expr.Name + el.Space1.Value + el.Trailing + "}}")
		}
	}
	return b.String()
}

// templateDisplay returns a template's decoded text with each placeholder
// shown as `{{name}}`.
func templateDisplay(t *syntax.Template) string {
	var b strings.Builder
	for _, el := range t.Elements {
		switch el := el.(type) {
		case *syntax.TemplateString:
			b.WriteString(el.Value)
		case *syntax.Placeholder:
			b.WriteString("{{" + el.Expr.Name + "}}")
		}
	}
	return b.String()
}
