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
	"regexp"
	"strconv"
	"strings"

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
		res, err = syntaxedit.AddEntry(name, src, syntax.EntrySpec{Method: op.Key, URL: syntax.PlainText(op.Value)})
	case RemoveEntry:
		res, err = syntaxedit.RemoveEntry(name, src, n)
	default:
		return nil, apperr.New(apperr.Invalid, "unknown edit "+op.Kind)
	}
	if err != nil {
		return nil, apperr.Wrap(apperr.Invalid, err)
	}
	return &Result{Version: b.Version, Edits: res.Edits, Text: string(res.Source)}, nil
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

// Methods lists an entry's gRPC methods.
func (s *Service) Methods(ctx context.Context, b Buffer, entry int, env string) ([]grpcx.Service, error) {
	return s.e.Methods(ctx, b, entry, env)
}
