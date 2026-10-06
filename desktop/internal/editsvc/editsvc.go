// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package editsvc edits a request file's buffer for the page: its model
// (entries, sections, rows), the entry at a cursor, structured edits
// (internal/syntaxedit), asserts and captures from a response body, and the
// gRPC methods of an entry. Offsets are UTF-16, as in the editor. Each
// result carries the buffer's version it was computed for: the page drops
// a result for an older version and asks again.
package editsvc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/nhtera/sonde/desktop/internal/apperr"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/enginex"
	"github.com/nhtera/sonde/internal/grpcx"
	"github.com/nhtera/sonde/internal/jsonpath"
	"github.com/nhtera/sonde/internal/syntax"
	"github.com/nhtera/sonde/internal/syntaxedit"
	"github.com/nhtera/sonde/internal/value"
)

// Buffer is the page's text of a file at a version.
type Buffer struct {
	File    string `json:"file"`
	Text    string `json:"text"`
	Version int    `json:"version"`
}

// Result is an edit of a buffer: the text edits (UTF-16 ranges of the
// buffer at Version) and the new text.
type Result struct {
	Version int                   `json:"version"`
	Edits   []syntaxedit.TextEdit `json:"edits"`
	Text    string                `json:"text"`
}

// Op is a structured edit. Kind picks it; the other fields are its
// arguments.
type Op struct {
	Kind    string `json:"kind"`
	Entry   int    `json:"entry"`
	Section string `json:"section"`
	Index   int    `json:"index"`
	Key     string `json:"key"`
	Value   string `json:"value"`
}

// Op kinds.
const (
	SetMethod     = "setMethod"     // Value
	SetURL        = "setURL"        // Value
	SetStatus     = "setStatus"     // Value
	SetRow        = "setRow"        // Section, Index, Key, Value
	AddRow        = "addRow"        // Section, Key, Value
	RemoveRow     = "removeRow"     // Section, Index
	ToggleRow     = "toggleRow"     // Section, Index
	EnsureSection = "ensureSection" // Section
	SetBody       = "setBody"       // Value
	AddAssertOp   = "addAssert"     // Value
	AddCaptureOp  = "addCapture"    // Key (name), Value (query)
	AddEntry      = "addEntry"      // Key (method), Value (URL)
	RemoveEntry   = "removeEntry"
	RemoveSection = "removeSection" // Section
	AddLogin      = "addLogin"      // Key (the token's capture name); a login request before Entry
	AppendEntries = "appendEntries" // Value: requests as text (an import), added at the end
	MoveURLQuery  = "moveURLQuery"  // the URL's query string to [Query] rows
)

// Bodies gives a stored response body (redacted).
type Bodies interface {
	Redacted(id string) ([]byte, string, bool)
}

// Preparer plans a file's run without running it.
type Preparer func(ctx context.Context, file, source, env string) (*engine.Runner, engine.Job, error)

// Edits is the edit service's core.
type Edits struct {
	bodies  Bodies
	prepare Preparer
}

// New returns the edit service.
func New(bodies Bodies, prepare Preparer) *Edits { return &Edits{bodies: bodies, prepare: prepare} }

// Model returns the entries of the buffer.
func (e *Edits) Model(b Buffer) ([]syntaxedit.EntryModel, error) {
	m, err := syntaxedit.Model(b.File, []byte(b.Text))
	if err != nil {
		return nil, apperr.Wrap(apperr.Invalid, err)
	}
	if m == nil {
		m = []syntaxedit.EntryModel{}
	}
	return m, nil
}

// EntryAt returns the 1-based entry at a UTF-16 offset of the buffer (0:
// none).
func (e *Edits) EntryAt(b Buffer, offset int) (int, error) {
	m, err := e.Model(b)
	if err != nil {
		return 0, err
	}
	return syntaxedit.EntryAt(m, offset), nil
}

// Apply runs op on the buffer.
func (e *Edits) Apply(b Buffer, op Op) (*Result, error) {
	name, src, n := b.File, []byte(b.Text), op.Entry
	sec := syntaxedit.Section(op.Section)
	var (
		res *syntaxedit.Result
		err error
	)
	switch op.Kind {
	case SetMethod:
		res, err = syntaxedit.SetMethod(name, src, n, op.Value)
	case SetURL:
		res, err = syntaxedit.SetURL(name, src, n, op.Value)
	case SetStatus:
		res, err = syntaxedit.SetStatus(name, src, n, op.Value)
	case SetRow:
		res, err = syntaxedit.SetRow(name, src, n, sec, op.Index, op.Key, op.Value)
	case AddRow:
		res, err = syntaxedit.AddRow(name, src, n, sec, op.Key, op.Value)
	case RemoveRow:
		res, err = syntaxedit.RemoveRow(name, src, n, sec, op.Index)
	case ToggleRow:
		res, err = syntaxedit.ToggleRow(name, src, n, sec, op.Index)
	case EnsureSection:
		res, err = syntaxedit.EnsureSection(name, src, n, sec)
	case SetBody:
		res, err = syntaxedit.SetBody(name, src, n, op.Value)
	case AddAssertOp:
		res, err = syntaxedit.AddAssert(name, src, n, op.Value)
	case AddCaptureOp:
		res, err = syntaxedit.AddCapture(name, src, n, op.Key, op.Value)
	case AddEntry:
		res, err = addEntry(name, src, op.Key, op.Value)
	case RemoveEntry:
		res, err = syntaxedit.RemoveEntry(name, src, n)
	case RemoveSection:
		res, err = syntaxedit.RemoveSection(name, src, n, sec)
	case AddLogin:
		res, err = syntaxedit.AddLoginEntry(name, src, n, loginSpec(op.Key))
	case AppendEntries:
		res, err = appendEntries(name, src, op.Value)
	case MoveURLQuery:
		res, err = syntaxedit.MoveURLQuery(name, src, n)
	default:
		return nil, apperr.New(apperr.Invalid, "unknown edit "+op.Kind)
	}
	if err != nil {
		return nil, apperr.Wrap(apperr.Invalid, err)
	}
	return &Result{Version: b.Version, Edits: res.Edits, Text: string(res.Source)}, nil
}

// addEntry appends a request whose URL is source text ({{variables}}
// kept): added with a plain URL, then given its own.
func addEntry(name string, src []byte, method, url string) (*syntaxedit.Result, error) {
	res, err := syntaxedit.AddEntry(name, src, syntax.EntrySpec{Method: method, URL: syntax.PlainText("http://new")})
	if err != nil {
		return nil, err
	}
	m, err := syntaxedit.Model(name, res.Source)
	if err != nil {
		return nil, err
	}
	res, err = syntaxedit.SetURL(name, res.Source, len(m), url)
	if err != nil {
		return nil, err
	}
	return &syntaxedit.Result{Source: res.Source, Edits: diff(string(src), string(res.Source))}, nil
}

// appendEntries adds requests (text) after the last one, a blank line
// between; the file must then have exactly those requests more.
func appendEntries(name string, src []byte, text string) (*syntaxedit.Result, error) {
	d := syntax.DialectFor(name)
	before, err := syntax.Parse(name, src, d)
	if err != nil {
		return nil, err
	}
	added, err := syntax.Parse(name, []byte(text), d)
	if err != nil {
		return nil, err
	}
	if len(added.Entries) == 0 {
		return nil, fmt.Errorf("no request to add")
	}
	out := string(src)
	if out != "" {
		out = strings.TrimRight(out, "\n") + "\n\n"
	}
	out += strings.TrimRight(text, "\n") + "\n"
	after, err := syntax.Parse(name, []byte(out), d)
	if err != nil || len(after.Entries) != len(before.Entries)+len(added.Entries) {
		return nil, fmt.Errorf("the requests do not add up: %v", err)
	}
	return &syntaxedit.Result{Source: []byte(out), Edits: diff(string(src), out)}, nil
}

// loginSpec is an OAuth 2 client-credentials login: its token captured
// (a secret) in capture ("token" by default), its endpoint and client in
// variables the user sets in the environment.
func loginSpec(capture string) syntaxedit.LoginSpec {
	if capture == "" {
		capture = "token"
	}
	v := func(name string) syntax.Text { return syntax.Text{syntax.Var(name)} }
	return syntaxedit.LoginSpec{
		Request: syntax.EntrySpec{
			Comments: []string{"Log in (OAuth 2 client credentials)"},
			Method:   "POST", URL: v("token_url"),
			Form: []syntax.Field{
				syntax.KV("grant_type", "client_credentials"),
				{Key: syntax.PlainText("client_id"), Value: v("client_id")},
				{Key: syntax.PlainText("client_secret"), Value: v("client_secret")},
			},
		},
		Capture: capture, JSONPath: "$.access_token", Redact: true,
	}
}

// Batch runs ops one after the other, each on the text the previous one
// left (one form action: a body kind change, an auth type change), and
// returns the whole change as one edit. Nothing is applied when an op
// fails.
func (e *Edits) Batch(b Buffer, ops []Op) (*Result, error) {
	text := b.Text
	for _, op := range ops {
		res, err := e.Apply(Buffer{File: b.File, Text: text, Version: b.Version}, op)
		if err != nil {
			return nil, err
		}
		text = res.Text
	}
	return &Result{Version: b.Version, Edits: diff(b.Text, text), Text: text}, nil
}

// diff is the edit from a to b: the part between their common prefix and
// suffix (UTF-16 offsets).
func diff(a, b string) []syntaxedit.TextEdit {
	if a == b {
		return []syntaxedit.TextEdit{}
	}
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	for p > 0 && p < len(a) && !utf8.RuneStart(a[p]) {
		p-- // a whole rune (at the end of a, p is one)
	}
	q := 0
	for q < len(a)-p && q < len(b)-p && a[len(a)-1-q] == b[len(b)-1-q] {
		q++
	}
	for q > 0 && !utf8.RuneStart(a[len(a)-q]) {
		q--
	}
	start := utf16Len(a[:p])
	return []syntaxedit.TextEdit{{Range: syntaxedit.Range{Start: start, End: start + utf16Len(a[p:len(a)-q])}, NewText: b[p : len(b)-q]}}
}

// Check is an assert split for the form's grid, as source text: its query
// (with its filters), its predicate (with `not`) and its value ("" for a
// predicate without one). Parsed is false when the text is not an assert
// (it then shows as text only).
type Check struct {
	Query     string `json:"query"`
	Predicate string `json:"predicate"`
	Value     string `json:"value"`
	Parsed    bool   `json:"parsed"`
}

// EntryChecks are the asserts of an entry, in the order of its asserts
// rows (disabled ones included).
type EntryChecks struct {
	Entry   int     `json:"entry"`
	Asserts []Check `json:"asserts"`
}

// Checks splits every assert row of the buffer for the form's grid.
func (e *Edits) Checks(b Buffer) ([]EntryChecks, error) {
	m, err := e.Model(b)
	if err != nil {
		return nil, err
	}
	out := make([]EntryChecks, 0, len(m))
	for _, en := range m {
		ec := EntryChecks{Entry: en.Index, Asserts: []Check{}}
		for _, r := range en.Rows[syntaxedit.Asserts] {
			ec.Asserts = append(ec.Asserts, splitAssert(b.File, r.Value))
		}
		out = append(out, ec)
	}
	return out, nil
}

// splitAssert splits an assert's text by parsing it in an entry of its
// own.
func splitAssert(file, text string) Check {
	const head = "GET http://x\nHTTP *\n[Asserts]\n"
	src := head + text + "\n"
	f, err := syntax.Parse(file, []byte(src), syntax.DialectFor(file))
	if err != nil || len(f.Entries) != 1 || f.Entries[0].Response == nil {
		return Check{Query: text}
	}
	for _, sec := range f.Entries[0].Response.Sections {
		if len(sec.Asserts) != 1 {
			continue
		}
		a := sec.Asserts[0]
		fn := a.Predicate.Func
		c := Check{Parsed: true, Query: src[a.Query.Span.Start.Offset:a.Space1.Span.Start.Offset]}
		if fn.Value != nil {
			c.Predicate = src[a.Space1.Span.End.Offset:fn.Space0.Span.Start.Offset]
			c.Value = src[fn.Space0.Span.End.Offset:fn.Span.End.Offset]
		} else {
			c.Predicate = src[a.Space1.Span.End.Offset:fn.Span.End.Offset]
		}
		return c
	}
	return Check{Query: text}
}

// FromBody asks for an assert or capture on a JSON response body.
type FromBody struct {
	Entry  int    `json:"entry"`
	BodyID string `json:"bodyId"`
	// Path is the JSONPath of the value, e.g. "$.items[0].id".
	Path string `json:"path"`
	// Capture, when set, adds a capture of that name instead of an assert.
	Capture string `json:"capture"`
}

// AssertValue adds an assert (or a capture) on the value at a JSONPath of
// a stored body. The value's text is rendered here, from the body: numbers
// keep every digit; a value the body shows masked is asserted to exist,
// never written into the file.
func (e *Edits) AssertValue(b Buffer, req FromBody) (*Result, error) {
	data, _, ok := e.bodies.Redacted(req.BodyID)
	if !ok {
		return nil, apperr.New(apperr.NotFound, "that response is no longer available; run the request again")
	}
	q, err := jsonpath.Parse(req.Path)
	if err != nil {
		return nil, apperr.Wrap(apperr.Invalid, err)
	}
	query := "jsonpath " + quote(req.Path)
	if req.Capture != "" {
		if !validName.MatchString(req.Capture) {
			return nil, apperr.New(apperr.Invalid, "not a variable name: "+req.Capture)
		}
		return e.Apply(b, Op{Kind: AddCaptureOp, Entry: req.Entry, Key: req.Capture, Value: query})
	}
	root, err := value.DecodeJSON(string(data))
	if err != nil {
		return nil, apperr.New(apperr.Invalid, "the body is not JSON")
	}
	v, ok := jsonpath.Unwrap(q.Eval(root))
	if !ok {
		return nil, apperr.New(apperr.NotFound, "no value at "+req.Path)
	}
	return e.Apply(b, Op{Kind: AddAssertOp, Entry: req.Entry, Value: query + " " + predicate(v)})
}

var validName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// predicate is the assert's predicate for v.
func predicate(v value.Value) string {
	switch x := v.(type) {
	case value.Null:
		return "== null"
	case value.Bool:
		return "== " + strconv.FormatBool(bool(x))
	case value.Int:
		return "== " + strconv.FormatInt(int64(x), 10)
	case value.BigInt:
		return "== " + string(x)
	case value.Float:
		return "== " + value.FormatFloat(float64(x))
	case value.String:
		if strings.Contains(string(x), "***") {
			return "exists" // masked: never write a secret's mask as its value
		}
		return "== " + quote(string(x))
	case value.List:
		return "count == " + strconv.Itoa(len(x))
	}
	return "exists"
}

// quote writes s as a quoted string of the file format: `{{` would start a
// variable, so its brace is escaped.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '{':
			if strings.HasPrefix(s[i+1:], "{") {
				b.WriteString(`\u{7b}`)
			} else {
				b.WriteRune(r)
			}
		default:
			if r < 0x20 {
				b.WriteString(`\u{` + strconv.FormatInt(int64(r), 16) + `}`)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// AppendMessages writes a message sent in an interactive session into
// the [SondeMessages] steps of an entry: `send: <message>` then
// `receive: 1`, before a final `close` step, else after the last step.
// Text is written as is when it is JSON, else as a `…` string; binary is
// hex digits.
func (e *Edits) AppendMessages(b Buffer, entry int, data string, binary bool) (*Result, error) {
	src := []byte(b.Text)
	f, err := syntax.Parse(b.File, src, syntax.DialectFor(b.File))
	if err != nil {
		return nil, apperr.Wrap(apperr.Invalid, err)
	}
	if entry < 1 || entry > len(f.Entries) {
		return nil, apperr.New(apperr.NotFound, "no request "+strconv.Itoa(entry))
	}
	var sec *syntax.Section
	for _, s := range f.Entries[entry-1].Request.Sections {
		if s.Kind == syntax.SectionMessages {
			sec = s
		}
	}
	if sec == nil {
		return nil, apperr.New(apperr.NotFound, "request "+strconv.Itoa(entry)+" has no [SondeMessages]")
	}
	value, err := messageValue(data, binary)
	if err != nil {
		return nil, err
	}
	nl := "\n"
	if strings.Contains(b.Text, "\r\n") {
		nl = "\r\n"
	}
	at := sec.LineTerminator0.Newline.Span.End.Offset
	if n := len(sec.Messages); n > 0 {
		last := sec.Messages[n-1]
		at = last.LineTerminator0.Newline.Span.End.Offset
		if last.Kind == syntax.StepClose {
			at = strings.LastIndexByte(b.Text[:last.Span.Start.Offset], '\n') + 1
		}
	}
	text := "send: " + value + nl + "receive: 1" + nl
	if at > 0 && src[at-1] != '\n' {
		text = nl + text // the step ends the file without a newline
	}
	out := b.Text[:at] + text + b.Text[at:]
	if _, err := syntax.Parse(b.File, []byte(out), syntax.DialectFor(b.File)); err != nil {
		return nil, apperr.New(apperr.Invalid, "this message cannot be written as a step: "+err.Error())
	}
	pos := utf16Len(b.Text[:at])
	return &Result{
		Version: b.Version,
		Edits:   []syntaxedit.TextEdit{{Range: syntaxedit.Range{Start: pos, End: pos}, NewText: text}},
		Text:    out,
	}, nil
}

// messageValue is the step value of a message.
func messageValue(data string, binary bool) (string, error) {
	if binary {
		digits := strings.Join(strings.Fields(data), "")
		if _, err := hex.DecodeString(digits); err != nil || digits == "" {
			return "", apperr.New(apperr.Invalid, "binary messages are hex digits, e.g. 01 ff")
		}
		return "hex," + digits + ";", nil
	}
	trimmed := strings.TrimSpace(data)
	if (strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) && json.Valid([]byte(trimmed)) && !strings.Contains(trimmed, "{{") {
		return trimmed, nil
	}
	if strings.ContainsAny(data, "`\\\n\r") || strings.Contains(data, "{{") {
		return "", apperr.New(apperr.Invalid, "only one-line text without backticks, backslashes or {{ can be written to the file")
	}
	return "`" + data + "`", nil
}

// utf16Len is the length of s in UTF-16 code units.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// Methods lists the gRPC services and methods an entry of the buffer can
// call (from the server's reflection or its proto files), in env.
func (e *Edits) Methods(ctx context.Context, b Buffer, entry int, env string) ([]grpcx.Service, error) {
	runner, job, err := e.prepare(ctx, b.File, b.Text, env)
	if err != nil {
		return nil, err
	}
	d, err := enginex.GRPCDescriptors(ctx, runner, job, entry)
	if err != nil {
		return nil, apperr.Wrap(apperr.Invalid, err)
	}
	if d == nil {
		return []grpcx.Service{}, nil // no proto named: reflection at run time
	}
	s := d.Services()
	if s == nil {
		s = []grpcx.Service{}
	}
	return s, nil
}

// Service is the edit bindings.
type Service struct{ e *Edits }

// NewService returns the bindings over e.
func NewService(e *Edits) *Service { return &Service{e: e} }

// Model returns the entries of the buffer.
func (s *Service) Model(b Buffer) ([]syntaxedit.EntryModel, error) { return s.e.Model(b) }

// EntryAt returns the entry at a UTF-16 offset (0: none).
func (s *Service) EntryAt(b Buffer, offset int) (int, error) { return s.e.EntryAt(b, offset) }

// Apply runs a structured edit.
func (s *Service) Apply(b Buffer, op Op) (*Result, error) { return s.e.Apply(b, op) }

// AssertValue adds an assert or capture on a response body's value.
func (s *Service) AssertValue(b Buffer, req FromBody) (*Result, error) {
	return s.e.AssertValue(b, req)
}

// EscapeFilename writes a path as a file name of the file format.
func (s *Service) EscapeFilename(path string) string { return syntax.EscapeFilename(path) }

// Batch runs several edits as one.
func (s *Service) Batch(b Buffer, ops []Op) (*Result, error) { return s.e.Batch(b, ops) }

// Checks splits the assert rows for the form's grid.
func (s *Service) Checks(b Buffer) ([]EntryChecks, error) { return s.e.Checks(b) }

// AppendMessages writes a session's message into an entry's steps.
func (s *Service) AppendMessages(b Buffer, entry int, data string, binary bool) (*Result, error) {
	return s.e.AppendMessages(b, entry, data, binary)
}

// Methods lists an entry's gRPC methods.
func (s *Service) Methods(ctx context.Context, b Buffer, entry int, env string) ([]grpcx.Service, error) {
	return s.e.Methods(ctx, b, entry, env)
}
