// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runsvc

import (
	"bytes"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/enginex"
	"github.com/nhtera/sonde/internal/syntaxedit"
)

// session is what a full run of a file leaves for Send: the captures of
// every entry (last writer wins), the cookie jar, and what it ran with.
// Send(n) reuses it only when entries 1…n-1, the environment, the
// overrides and the data row are the same.
type session struct {
	src []byte
	env string
	// values is a digest of what the run resolved (project values,
	// overrides), so an edited sonde.yaml does not match.
	values string
	row    int
	// last is the last entry that ran: Send(n) needs 1…n-1.
	last     int
	captures []capture
	cookies  []engine.Cookie
	at       time.Time
}

type capture struct {
	name   string
	value  engine.Value
	secret bool
}

// newSession records a finished run.
func newSession(src []byte, env, values string, res *engine.UnitResult) *session {
	s := &session{src: src, env: env, values: values, row: res.Row, at: res.Timestamp}
	if s.at.IsZero() {
		s.at = time.Now()
	}
	s.merge(res)
	return s
}

// merge adds the captures and cookies of res (a run or a Send).
func (s *session) merge(res *engine.UnitResult) {
	for i, e := range res.Entries {
		for j, c := range e.Captures {
			s.captures = append(s.captures, capture{name: c.Name, value: c.Value, secret: enginex.CaptureRedacted(res, i, j)})
		}
	}
	s.cookies = append([]engine.Cookie(nil), res.Cookies...)
	if n := len(res.Entries); n > 0 && res.Entries[n-1].Index > s.last {
		s.last = res.Entries[n-1].Index
	}
}

// layers splits the captures for runplan.ApplyCaptures, last writer
// winning: a name captured plain after it was captured redacted is plain.
func (s *session) layers() (plain map[string]any, secret map[string]string) {
	plain, secret = map[string]any{}, map[string]string{}
	for _, c := range s.captures {
		if c.secret {
			delete(plain, c.name)
			text, ok := c.value.Text()
			if !ok {
				text = c.value.String()
			}
			secret[c.name] = text
		} else {
			delete(secret, c.name)
			plain[c.name] = c.value
		}
	}
	return plain, secret
}

// matches reports whether a Send of entry n of src (env, overrides, row)
// may reuse the session.
func (s *session) matches(src []byte, n int, env, values string, row int) bool {
	if s.env != env || s.values != values || s.row != row || s.last < n-1 {
		return false
	}
	a, okA := prefix(s.src, n)
	b, okB := prefix(src, n)
	return okA && okB && bytes.Equal(a, b)
}

// prefix returns the source before entry n (entries 1…n-1 and what
// precedes them).
func prefix(src []byte, n int) ([]byte, bool) {
	models, err := syntaxedit.Model("", src)
	if err != nil || n < 1 || n > len(models) {
		return nil, false
	}
	return src[:byteOffset(src, models[n-1].Range.Start)], true
}

// byteOffset converts a UTF-16 offset of src to a byte offset.
func byteOffset(src []byte, units int) int {
	i := 0
	for i < len(src) && units > 0 {
		r, size := utf8.DecodeRune(src[i:])
		units -= utf16.RuneLen(r)
		i += size
	}
	return i
}
