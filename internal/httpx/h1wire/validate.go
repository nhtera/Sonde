// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package h1wire is Sonde's own HTTP/1.x client: it writes requests with
// the version, header order and header case they are given, and reads
// responses leniently (header names net/http refuses, wire order and
// case kept) but frames them strictly. A connection that saw anything
// unusual is never reused. HTTP/2 stays on net/http.
package h1wire

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

// ASCIIHost is host with an internationalized name in its ASCII
// (punycode) form, as it goes on the wire (Host header, dial address,
// TLS server name); host itself when it is ASCII or cannot be converted.
func ASCIIHost(host string) string {
	for i := 0; i < len(host); i++ {
		if host[i] >= utf8.RuneSelf {
			if a, err := idna.Lookup.ToASCII(host); err == nil {
				return a
			}
			return host
		}
	}
	return host
}

// isTokenChar reports whether c may appear in an RFC 9110 token.
func isTokenChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	return strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0
}

// isToken reports whether s is a non-empty RFC 9110 token.
func isToken(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isTokenChar(s[i]) {
			return false
		}
	}
	return true
}

// validFieldValue reports whether v holds no control character other
// than a horizontal tab (CR and LF included: no header injection).
func validFieldValue(v string) bool {
	for i := 0; i < len(v); i++ {
		if c := v[i]; (c < 0x20 && c != '\t') || c == 0x7f {
			return false
		}
	}
	return true
}

// validTarget reports whether a request target has no space and no
// control character.
func validTarget(t string) bool {
	if t == "" {
		return false
	}
	for i := 0; i < len(t); i++ {
		if c := t[i]; c <= 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}

// RequestError is a request sonde refuses to write: a method, target or
// header that would not be read back as written.
type RequestError struct{ Msg string }

func (e *RequestError) Error() string { return e.Msg }

func requestErrorf(format string, args ...any) error {
	return &RequestError{Msg: fmt.Sprintf(format, args...)}
}
