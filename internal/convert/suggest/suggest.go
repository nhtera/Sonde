// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package suggest reads the test scripts of an imported collection (never
// executed) as edits to its files, for a user to accept one by one:
// asserts and captures. A script statement is translated only when it has
// exactly one of a few known forms; anything else stays in the file's
// comments. The forms are the same for every collection format, spelled
// in the format's own script API (an API).
package suggest

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntaxedit"
)

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

// Labels of the suggestions read from scripts.
const (
	AssertsLabel  = "Translate test script assertions"
	CapturesLabel = "Capture variables set by test scripts"
)

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

// API spells a script API's calls, each a regular expression matching the
// call as normalize writes it.
type API struct {
	Test   string // the test function, before its `("name", callback)`
	Expect string // the assertion function, before its `(value)`
	Body   string // the response body parsed as JSON
	Status string // the response status code
	// HeaderGet reads a response header, before its `(name)`.
	HeaderGet string
	// HeaderHas asserts a response header exists, before its `(name)`;
	// "" when the API has no such call.
	HeaderHas string
	// Set sets a variable, before its `("name", value)`.
	Set string
}

// Reader reads the scripts of one API.
type Reader struct {
	alias, expectEq, headerHas, headerGet, set, body, status, testCallback, testExpr, success *regexp.Regexp
}

// NewReader compiles api; it panics on an invalid expression, as the APIs
// are constants of the importers.
func NewReader(api API) *Reader {
	r := &Reader{
		alias:        regexp.MustCompile(`^(?:var|let|const) ([A-Za-z_$][\w$]*) = (?:` + api.Body + `)$`),
		expectEq:     regexp.MustCompile(`^(?:` + api.Expect + `)\((.+)\)\.to\.(?:eql|equal|deep\.equal|be\.equal)\((.+)\)$`),
		headerGet:    regexp.MustCompile(`^(?:` + api.HeaderGet + `)\((.+)\)$`),
		set:          regexp.MustCompile(`^(?:` + api.Set + `)\(("[^"]*"|'[^']*'), (.+)\)$`),
		body:         regexp.MustCompile(`^(?:` + api.Body + `)`),
		status:       regexp.MustCompile(`^(?:` + api.Status + `)$`),
		testCallback: regexp.MustCompile(`^(?:` + api.Test + `)\(("[^"]*"|'[^']*'), (?:async )?(?:function\(\)|\(\) =>)$`),
		testExpr:     regexp.MustCompile(`^(?:` + api.Test + `)\((?:"[^"]*"|'[^']*'), (?:async )?\(\) => (.+)\)$`),
		success:      regexp.MustCompile(`^if\((?:` + api.Status + `) ?(<=|<|===|==) ?(\d+)\)$`),
	}
	if api.HeaderHas != "" {
		r.headerHas = regexp.MustCompile(`^(?:` + api.HeaderHas + `)\((.+)\)$`)
	}
	return r
}

var (
	assignRE = regexp.MustCompile(`^(?:(?:var|let|const) )?([A-Za-z_$][\w$]*) = `)
	accessRE = regexp.MustCompile(`^(?:\.([A-Za-z_$][\w$]*)|\[(\d+)\]|\[("[^"]*"|'[^']*')\])`)
)

// Read translates the statements of a script it recognizes to assert and
// capture rows (Entry unset).
func (r *Reader) Read(code string) (asserts, captures []Op) {
	aliases := map[string]bool{}
	for _, st := range r.statements(code) {
		stmt := st.text
		// test("…", () => expr): expr runs, as in a callback's block.
		if m := r.testExpr.FindStringSubmatch(stmt); m != nil {
			stmt = m[1]
		}
		if m := r.alias.FindStringSubmatch(stmt); m != nil {
			aliases[m[1]] = true
			continue
		}
		// Any other assignment of an alias ends it.
		if m := assignRE.FindStringSubmatch(stmt); m != nil {
			delete(aliases, m[1])
			continue
		}
		// Only a capture runs on success alone: an assertion there checks
		// nothing when the request fails.
		if st.onSuccess && !r.set.MatchString(stmt) {
			continue
		}
		if r.headerHas != nil {
			if m := r.headerHas.FindStringSubmatch(stmt); m != nil {
				if name, ok := jsQuoted(m[1]); ok {
					asserts = append(asserts, Op{Section: syntaxedit.Asserts, Value: "header " + name + " exists"})
				}
				continue
			}
		}
		if m := r.expectEq.FindStringSubmatch(stmt); m != nil {
			want, ok := literal(m[2])
			if !ok {
				continue
			}
			if h := r.headerGet.FindStringSubmatch(m[1]); h != nil {
				if name, ok := jsQuoted(h[1]); ok {
					asserts = append(asserts, Op{Section: syntaxedit.Asserts, Value: "header " + name + " == " + want})
				}
				continue
			}
			if r.status.MatchString(m[1]) {
				asserts = append(asserts, Op{Section: syntaxedit.Asserts, Value: "status == " + want})
				continue
			}
			if path, ok := r.jsonPath(m[1], aliases); ok {
				if q, ok := quote(path); ok {
					asserts = append(asserts, Op{Section: syntaxedit.Asserts, Value: "jsonpath " + q + " == " + want})
				}
			}
			continue
		}
		if m := r.set.FindStringSubmatch(stmt); m != nil {
			raw, _ := jsString(m[1])
			name := convert.VariableName(raw)
			if path, ok := r.jsonPath(m[2], aliases); ok && name != "" {
				if q, ok := quote(path); ok {
					captures = append(captures, Op{Section: syntaxedit.Captures, Key: name, Value: "jsonpath " + q})
				}
			}
		}
	}
	return asserts, captures
}

// Entry is the asserts and captures read for one entry of a file.
type Entry struct{ Asserts, Captures []Op }

// Suggestions turns the asserts and captures of each entry (1-based) of
// file into suggestions, one per kind.
func Suggestions(file int, entries []Entry) []FileSuggestion {
	var asserts, captures []Op
	for i, e := range entries {
		for _, op := range e.Asserts {
			op.Entry = i + 1
			asserts = append(asserts, op)
		}
		for _, op := range e.Captures {
			op.Entry = i + 1
			captures = append(captures, op)
		}
	}
	var out []FileSuggestion
	if len(asserts) > 0 {
		out = append(out, FileSuggestion{File: file, Label: AssertsLabel, Ops: asserts})
	}
	if len(captures) > 0 {
		out = append(out, FileSuggestion{File: file, Label: CapturesLabel, Ops: captures})
	}
	return out
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

// statement is a statement of a script, and whether it runs only when the
// response is a success.
type statement struct {
	text      string
	onSuccess bool
}

// How the statements of a block run.
const (
	always = iota
	onSuccess
	never
)

// statements returns the statements of a script that run whenever the
// script does (at its top level or directly in a test callback) or when
// the response is a success (in an `if` on a 2xx status), never inside
// another if, a loop, a function or a skipped test. Comments are dropped,
// strings kept as written, and each statement normalized. A statement is
// ended by `;`, a brace, or a line break not followed by a `.` (a chained
// call on the next line).
func (r *Reader) statements(code string) []statement {
	var out []statement
	var blocks []int // how each open block runs its statements
	var cur strings.Builder
	runs := func() int {
		mode := always
		for _, b := range blocks {
			mode = max(mode, b)
		}
		return mode
	}
	flush := func() {
		if s := normalize(cur.String()); s != "" && runs() != never {
			out = append(out, statement{text: s, onSuccess: runs() == onSuccess})
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
			mode := never
			switch {
			case r.testCallback.MatchString(head):
				mode = runs()
			case r.isSuccess(head):
				mode = max(runs(), onSuccess)
			}
			blocks = append(blocks, mode)
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

// isSuccess reports whether head, an if before its block, holds when the
// status is 2xx and only then: status < N (201..300), <= N (200..299), or
// == N (200..299).
func (r *Reader) isSuccess(head string) bool {
	m := r.success.FindStringSubmatch(head)
	if m == nil {
		return false
	}
	n, _ := strconv.Atoi(m[2])
	if m[1] == "<" {
		return n > 200 && n <= 300
	}
	return n >= 200 && n < 300
}

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
			if strings.HasSuffix(out, "=") || strings.HasSuffix(out, "!") || strings.HasSuffix(out, "<") || strings.HasSuffix(out, ">") || i+1 < len(s) && s[i+1] == '=' {
				b.WriteByte('=') // part of ==, ===, !=, <=, >=
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

// jsonPath converts the API's JSON body, or an alias of it, followed by
// property and index accessors, to a JSONPath.
func (r *Reader) jsonPath(expr string, aliases map[string]bool) (string, bool) {
	var rest string
	if loc := r.body.FindStringIndex(expr); loc != nil {
		rest = expr[loc[1]:]
	} else {
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
