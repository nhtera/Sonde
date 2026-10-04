// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package postman

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/syntaxedit"
)

// Suggestions are edits an import proposes on top of its files, for a
// user to accept one by one: asserts and captures read from the test
// scripts of the collection (never executed), and a login entry for
// OAuth2. A script statement is translated only when it has exactly one
// of a few known forms; anything else stays in the file's comments.

// Op is one edit of a suggestion: a row added to Entry (1-based entry of
// the file), or a login entry inserted before Entry.
type Op struct {
	Entry      int
	Section    syntaxedit.Section
	Key, Value string // the row, source text
	Login      *syntaxedit.LoginSpec
}

// FileSuggestion is a suggestion for one file of an import: File is its
// index in the Output.Files of the same import.
type FileSuggestion struct {
	File  int
	Label string
	Ops   []Op
}

// Apply applies the suggestion to src, the file's content (name gives its
// dialect): rows first, then login entries, last entry first, so that no
// insertion moves the entry an op addresses.
func (s FileSuggestion) Apply(name string, src []byte) ([]byte, error) {
	ops := append([]Op(nil), s.Ops...)
	sort.SliceStable(ops, func(i, j int) bool {
		li, lj := ops[i].Login != nil, ops[j].Login != nil
		if li != lj {
			return !li
		}
		return li && ops[i].Entry > ops[j].Entry
	})
	for _, op := range ops {
		var res *syntaxedit.Result
		var err error
		if op.Login != nil {
			res, err = syntaxedit.AddLoginEntry(name, src, op.Entry, *op.Login)
		} else {
			res, err = syntaxedit.ApplyEdits(name, src, []syntaxedit.RowInsert{{Entry: op.Entry, Section: op.Section, Key: op.Key, Value: op.Value}})
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.Label, err)
		}
		src = res.Source
	}
	return src, nil
}

// Suggest returns the suggestions of the import of data under opts: the
// same collection walk as Import, whose Output.Files the suggestions'
// File indices address.
func Suggest(data []byte, opts Options) (out []FileSuggestion, err error) {
	defer func() {
		if p := recover(); p != nil {
			out, err = nil, fmt.Errorf("postman: malformed collection: %v", p)
		}
	}()
	w, err := walk(data, opts)
	if err != nil {
		return nil, err
	}
	return w.suggestions, nil
}

// entryOps are the suggestions of one entry, before its file is known.
type entryOps struct {
	asserts, captures []Op
	oauth2            *syntaxedit.LoginSpec
}

// fileSuggestions turns the ops of each entry (1-based) of file into
// suggestions, one per kind.
func (w *walker) fileSuggestions(file int, entries []entryOps) {
	var asserts, captures, login []Op
	for i, e := range entries {
		n := i + 1
		for _, op := range e.asserts {
			op.Entry = n
			asserts = append(asserts, op)
		}
		for _, op := range e.captures {
			op.Entry = n
			captures = append(captures, op)
		}
		if e.oauth2 != nil {
			if len(login) == 0 {
				login = append(login, Op{Entry: n, Login: e.oauth2})
			}
			login = append(login, Op{Entry: n, Section: syntaxedit.Headers, Key: "Authorization", Value: "Bearer {{" + e.oauth2.Capture + "}}"})
		}
	}
	add := func(label string, ops []Op) {
		if len(ops) > 0 {
			w.suggestions = append(w.suggestions, FileSuggestion{File: file, Label: label, Ops: ops})
		}
	}
	add("Translate test script assertions", asserts)
	add("Capture variables set by test scripts", captures)
	add("Log in with OAuth2 and send the token", login)
}

// entryOps reads the suggestions of a request item: its test scripts,
// and its OAuth2 auth.
func (w *walker) entryOps(it item, a authState) entryOps {
	var ops entryOps
	ops.oauth2 = oauth2Login(a.auth)
	for _, ev := range it.Event {
		if ev.Disabled || ev.Listen != "test" {
			continue
		}
		readScript(scriptCode(ev.Script), &ops)
	}
	return ops
}

var (
	assignRE    = regexp.MustCompile(`^(?:(?:var|let|const) )?([A-Za-z_$][\w$]*) = `)
	aliasRE     = regexp.MustCompile(`^(?:var|let|const) ([A-Za-z_$][\w$]*) = pm\.response\.json\(\)$`)
	expectEqRE  = regexp.MustCompile(`^pm\.expect\((.+)\)\.to\.(?:eql|equal|deep\.equal|be\.equal)\((.+)\)$`)
	headerHasRE = regexp.MustCompile(`^pm\.response\.to\.have\.header\((.+)\)$`)
	headerGetRE = regexp.MustCompile(`^pm\.response\.headers\.get\((.+)\)$`)
	setRE       = regexp.MustCompile(`^pm\.(?:environment|collectionVariables|globals|variables)\.set\(("[^"]*"|'[^']*'), (.+)\)$`)
	accessRE    = regexp.MustCompile(`^(?:\.([A-Za-z_$][\w$]*)|\[(\d+)\]|\[("[^"]*"|'[^']*')\])`)
)

// readScript translates the statements of a script it recognizes.
func readScript(code string, ops *entryOps) {
	aliases := map[string]bool{}
	for _, stmt := range statements(code) {
		// pm.test("…", () => expr): expr runs, as in a callback's block.
		if m := testExprRE.FindStringSubmatch(stmt); m != nil {
			stmt = m[1]
		}
		if m := aliasRE.FindStringSubmatch(stmt); m != nil {
			aliases[m[1]] = true
			continue
		}
		// Any other assignment of an alias ends it.
		if m := assignRE.FindStringSubmatch(stmt); m != nil {
			delete(aliases, m[1])
			continue
		}
		if m := headerHasRE.FindStringSubmatch(stmt); m != nil {
			if name, ok := jsQuoted(m[1]); ok {
				ops.asserts = append(ops.asserts, Op{Section: syntaxedit.Asserts, Value: "header " + name + " exists"})
			}
			continue
		}
		if m := expectEqRE.FindStringSubmatch(stmt); m != nil {
			want, ok := literal(m[2])
			if !ok {
				continue
			}
			if h := headerGetRE.FindStringSubmatch(m[1]); h != nil {
				if name, ok := jsQuoted(h[1]); ok {
					ops.asserts = append(ops.asserts, Op{Section: syntaxedit.Asserts, Value: "header " + name + " == " + want})
				}
				continue
			}
			if m[1] == "pm.response.code" {
				ops.asserts = append(ops.asserts, Op{Section: syntaxedit.Asserts, Value: "status == " + want})
				continue
			}
			if path, ok := jsonPath(m[1], aliases); ok {
				if q, ok := quote(path); ok {
					ops.asserts = append(ops.asserts, Op{Section: syntaxedit.Asserts, Value: "jsonpath " + q + " == " + want})
				}
			}
			continue
		}
		if m := setRE.FindStringSubmatch(stmt); m != nil {
			raw, _ := jsString(m[1])
			name := convert.VariableName(raw)
			if path, ok := jsonPath(m[2], aliases); ok && name != "" {
				if q, ok := quote(path); ok {
					ops.captures = append(ops.captures, Op{Section: syntaxedit.Captures, Key: name, Value: "jsonpath " + q})
				}
			}
		}
	}
}

// jsQuoted is a JavaScript string literal as a quoted string of a request
// file.
func jsQuoted(lit string) (string, bool) {
	s, ok := jsString(lit)
	if !ok {
		return "", false
	}
	return quote(s)
}

// statements returns the statements of a script that run whenever the
// script does: at its top level or directly in a pm.test callback, never
// inside an if, a loop, a function or a skipped test. Comments are
// dropped, strings kept as written, and each statement normalized. A
// statement is ended by `;`, a brace, or a line break not followed by a
// `.` (a chained call on the next line).
func statements(code string) []string {
	var out []string
	var blocks []bool // whether each open block runs its statements
	var cur strings.Builder
	runs := func() bool {
		for _, b := range blocks {
			if !b {
				return false
			}
		}
		return true
	}
	flush := func() {
		if s := normalize(cur.String()); s != "" && runs() {
			out = append(out, s)
		}
		cur.Reset()
	}
	for i := 0; i < len(code); i++ {
		c := code[i]
		switch {
		case c == '/' && i+1 < len(code) && code[i+1] == '/':
			for i < len(code) && code[i] != '\n' {
				i++
			}
			i--
		case c == '/' && i+1 < len(code) && code[i+1] == '*':
			end := strings.Index(code[i+2:], "*/")
			if end < 0 {
				i = len(code)
				break
			}
			i += 2 + end + 1
			cur.WriteByte(' ')
		case c == '"' || c == '\'' || c == '`':
			j := i + 1
			for j < len(code) && code[j] != c {
				if code[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(code))
			cur.WriteString(code[i:j])
			i = j - 1
		case c == ';':
			flush()
		case c == '\n':
			rest := strings.TrimLeft(code[i+1:], " \t\r\n")
			if strings.HasPrefix(rest, ".") {
				cur.WriteByte(' ')
			} else {
				flush()
			}
		case c == '{':
			head := normalize(cur.String())
			cur.Reset()
			blocks = append(blocks, runs() && testCallbackRE.MatchString(head))
		case c == '}':
			flush()
			if len(blocks) > 0 {
				blocks = blocks[:len(blocks)-1]
			}
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return out
}

// testCallbackRE is the start of a pm.test callback, before its `{`.
var testCallbackRE = regexp.MustCompile(`^pm\.test\(("[^"]*"|'[^']*'), (?:async )?(?:function\(\)|\(\) =>)$`)

// testExprRE is a pm.test whose callback is an arrow function's
// expression; it captures the expression.
var testExprRE = regexp.MustCompile(`^pm\.test\((?:"[^"]*"|'[^']*'), (?:async )?\(\) => (.+)\)$`)

// normalize writes a statement with no space around `(`, `)`, `.`, `[`,
// `]`, one after `,` and around `=`, and none at its ends; the text of
// string literals is kept as is.
func normalize(s string) string {
	var b strings.Builder
	space := false // a space is pending
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\'':
			j := i + 1
			for j < len(s) && s[j] != c {
				if s[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(s))
			if space && spaceBefore(&b) {
				b.WriteByte(' ')
			}
			b.WriteString(s[i:j])
			i, space = j-1, false
		case c == ' ' || c == '\t' || c == '\r':
			space = true
		case strings.IndexByte("().[]", c) >= 0:
			b.WriteByte(c)
			space = false
		case c == ',':
			b.WriteString(", ")
			space = false
		case c == '=' && i+1 < len(s) && s[i+1] == '>':
			out := strings.TrimRight(b.String(), " ")
			b.Reset()
			b.WriteString(out + " =>")
			i++
			space = true
		case c == '=':
			out := strings.TrimRight(b.String(), " ")
			b.Reset()
			b.WriteString(out)
			if strings.HasSuffix(out, "=") || strings.HasSuffix(out, "!") || i+1 < len(s) && s[i+1] == '=' {
				b.WriteByte('=') // part of ==, ===, !=
			} else {
				b.WriteString(" = ")
			}
			space = false
		default:
			if space && spaceBefore(&b) {
				b.WriteByte(' ')
			}
			b.WriteByte(c)
			space = false
		}
	}
	return strings.TrimSpace(b.String())
}

// spaceBefore reports whether a pending space is written after what b
// holds: not at the start, nor after punctuation or an operator.
func spaceBefore(b *strings.Builder) bool {
	if b.Len() == 0 {
		return false
	}
	return strings.IndexByte("().[] =!", b.String()[b.Len()-1]) < 0
}

// jsonPath converts `pm.response.json()` or an alias of it, followed by
// property and index accessors, to a JSONPath.
func jsonPath(expr string, aliases map[string]bool) (string, bool) {
	rest, ok := strings.CutPrefix(expr, "pm.response.json()")
	if !ok {
		name := expr
		if i := strings.IndexAny(expr, ".["); i >= 0 {
			name = expr[:i]
		}
		if !aliases[name] {
			return "", false
		}
		rest = expr[len(name):]
	}
	path := "$"
	for rest != "" {
		m := accessRE.FindStringSubmatch(rest)
		if m == nil {
			return "", false
		}
		switch {
		case m[1] != "":
			path += "." + m[1]
		case m[2] != "":
			path += "[" + m[2] + "]"
		default:
			key, _ := jsString(m[3])
			path += "['" + strings.ReplaceAll(key, "'", `\'`) + "']"
		}
		rest = rest[len(m[0]):]
	}
	return path, true
}

// jsString decodes a JavaScript string literal in single or double quotes
// (without escapes).
func jsString(s string) (string, bool) {
	if len(s) < 2 || (s[0] != '"' && s[0] != '\'') || s[len(s)-1] != s[0] || strings.ContainsAny(s[1:len(s)-1], `\`+s[:1]) {
		return "", false
	}
	return s[1 : len(s)-1], true
}

// literal converts a JavaScript literal (number, string, boolean, null) to
// a predicate value.
func literal(s string) (string, bool) {
	switch s {
	case "true", "false", "null":
		return s, true
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil && !strings.ContainsAny(s, "xXeE_") {
		return s, true
	}
	return jsQuoted(s)
}

// quote writes s as a double-quoted string of a request file; ok is false
// for a string one can not hold ({{ starts a placeholder).
func quote(s string) (string, bool) {
	if strings.Contains(s, "{{") {
		return "", false
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u{%x}`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String(), true
}

// oauth2Login reads an oauth2 auth as a login entry capturing
// access_token, for the grant types that are one form post.
func oauth2Login(a *auth) *syntaxedit.LoginSpec {
	if a == nil || a.Type != "oauth2" {
		return nil
	}
	tokenURL := paramValue(a.Oauth2, "accessTokenUrl")
	grant := paramValue(a.Oauth2, "grant_type")
	if tokenURL == "" {
		return nil
	}
	fields := map[string]string{
		"client_credentials": "client_id=clientId client_secret=clientSecret scope=scope",
		"password":           "client_id=clientId client_secret=clientSecret username=username password=password scope=scope",
	}[grant]
	if fields == "" {
		return nil
	}
	urlText, _ := convert.ParseText(tokenURL)
	form := []syntax.Field{syntax.KV("grant_type", grant)}
	for _, f := range strings.Fields(fields) {
		key, param, _ := strings.Cut(f, "=")
		if v := paramValue(a.Oauth2, param); v != "" {
			vt, _ := convert.ParseText(v)
			form = append(form, syntax.Field{Key: syntax.PlainText(key), Value: vt})
		}
	}
	return &syntaxedit.LoginSpec{
		Request:  syntax.EntrySpec{Comments: []string{"OAuth2 " + grant + " login"}, Method: "POST", URL: urlText, Form: form},
		Capture:  "access_token",
		JSONPath: "$.access_token",
		Redact:   true,
	}
}
