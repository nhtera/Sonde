// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"encoding/base64"
	"encoding/hex"
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/nhtera/sonde/internal/charset"
	"github.com/nhtera/sonde/internal/runerr"
	"github.com/nhtera/sonde/internal/value"
)

func (c call) base64Decode(v value.Value) (value.Value, error) {
	s, ok := v.(value.String)
	if !ok {
		return nil, c.typeError(v, "base64 string")
	}
	b, err := decodeBase64(base64.StdEncoding.Strict(), string(s))
	if err != nil {
		return nil, c.invalidValue("string is not base64")
	}
	return value.Bytes(b), nil
}

func (c call) base64URLSafeDecode(v value.Value) (value.Value, error) {
	s, ok := v.(value.String)
	if !ok {
		return nil, c.typeError(v, "string")
	}
	if trimmed := strings.TrimRight(string(s), "="); len(trimmed) < len(s) && !strings.Contains(trimmed, "=") {
		return nil, c.invalidValue("base64 string contains padding")
	}
	b, err := decodeBase64(base64.RawURLEncoding.Strict(), string(s))
	if err != nil {
		return nil, c.invalidValue("string is not base64")
	}
	return value.Bytes(b), nil
}

// decodeBase64 decodes s; unlike the encoding package, line breaks are
// not skipped.
func decodeBase64(enc *base64.Encoding, s string) ([]byte, error) {
	if strings.ContainsAny(s, "\r\n") {
		return nil, base64.CorruptInputError(strings.IndexAny(s, "\r\n"))
	}
	return enc.DecodeString(s)
}

func (c call) base64Encode(v value.Value) (value.Value, error) {
	b, ok := v.(value.Bytes)
	if !ok {
		return nil, c.typeError(v, "bytes")
	}
	return value.String(base64.StdEncoding.EncodeToString(b)), nil
}

func (c call) base64URLSafeEncode(v value.Value) (value.Value, error) {
	b, ok := v.(value.Bytes)
	if !ok {
		return nil, c.typeError(v, "bytes")
	}
	return value.String(base64.RawURLEncoding.EncodeToString(b)), nil
}

func (c call) toHex(v value.Value) (value.Value, error) {
	b, ok := v.(value.Bytes)
	if !ok {
		return nil, c.typeError(v, "bytes")
	}
	return value.String(hex.EncodeToString(b)), nil
}

func (c call) charsetDecode(v value.Value) (value.Value, error) {
	label, err := c.arg()
	if err != nil {
		return nil, err
	}
	b, ok := v.(value.Bytes)
	if !ok {
		return nil, c.typeError(v, "bytes")
	}
	enc, ok := charset.Lookup(label)
	if !ok {
		e := runerr.New(c.f.Span, runerr.FilterInvalidEncoding, c.assert)
		e.Value = label
		return nil, e
	}
	s, ok := charset.Decode(enc, b)
	if !ok {
		e := runerr.New(c.f.Span, runerr.FilterDecode, c.assert)
		e.Value = label
		return nil, e
	}
	return value.String(s), nil
}

func (c call) utf8Decode(v value.Value) (value.Value, error) {
	b, ok := v.(value.Bytes)
	if !ok {
		return nil, c.typeError(v, "bytes")
	}
	return value.String(lossyUTF8(b)), nil
}

func (c call) utf8Encode(v value.Value) (value.Value, error) {
	s, ok := v.(value.String)
	if !ok {
		return nil, c.typeError(v, "string")
	}
	return value.Bytes(s), nil
}

// lossyUTF8 decodes b, replacing each maximal invalid subsequence with one
// U+FFFD (the WHATWG and Unicode recommended practice).
func lossyUTF8(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if r != utf8.RuneError || size > 1 {
			sb.WriteRune(r)
			b = b[size:]
			continue
		}
		sb.WriteRune(utf8.RuneError)
		b = b[invalidPrefix(b):]
	}
	return sb.String()
}

// invalidPrefix returns the length of the maximal subpart of an ill-formed
// sequence at the start of b (at least 1).
func invalidPrefix(b []byte) int {
	lo, hi := byte(0x80), byte(0xBF)
	var need int
	switch c := b[0]; {
	case c >= 0xC2 && c <= 0xDF:
		need = 1
	case c == 0xE0:
		need, lo = 2, 0xA0
	case c >= 0xE1 && c <= 0xEC, c == 0xEE, c == 0xEF:
		need = 2
	case c == 0xED:
		need, hi = 2, 0x9F
	case c == 0xF0:
		need, lo = 3, 0x90
	case c >= 0xF1 && c <= 0xF3:
		need = 3
	case c == 0xF4:
		need, hi = 3, 0x8F
	default:
		return 1
	}
	n := 1
	for ; n <= need && n < len(b); n++ {
		if b[n] < lo || b[n] > hi {
			break
		}
		lo, hi = 0x80, 0xBF
	}
	return n
}

func (c call) urlEncode(v value.Value) (value.Value, error) {
	s, ok := v.(value.String)
	if !ok {
		return nil, c.typeError(v, "string")
	}
	const upperHex = "0123456789ABCDEF"
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || strings.IndexByte("-._~/", b) >= 0 {
			sb.WriteByte(b)
			continue
		}
		sb.WriteByte('%')
		sb.WriteByte(upperHex[b>>4])
		sb.WriteByte(upperHex[b&0x0f])
	}
	return value.String(sb.String()), nil
}

func (c call) urlDecode(v value.Value) (value.Value, error) {
	s, ok := v.(value.String)
	if !ok {
		return nil, c.typeError(v, "string")
	}
	return value.String(percentDecode(string(s), false)), nil
}

// percentDecode decodes %XX escapes (malformed ones are kept as is) and,
// for form encoding, '+' as a space; invalid UTF-8 is replaced.
func percentDecode(s string, form bool) string {
	if !strings.ContainsAny(s, "%+") {
		return s
	}
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]):
			n, _ := strconv.ParseUint(s[i+1:i+3], 16, 8)
			b = append(b, byte(n))
			i += 2
		case s[i] == '+' && form:
			b = append(b, ' ')
		default:
			b = append(b, s[i])
		}
	}
	return lossyUTF8(b)
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func (c call) htmlEscape(v value.Value) (value.Value, error) {
	s, ok := v.(value.String)
	if !ok {
		return nil, c.typeError(v, "string")
	}
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#x27;")
	return value.String(r.Replace(string(s))), nil
}

func (c call) htmlUnescape(v value.Value) (value.Value, error) {
	s, ok := v.(value.String)
	if !ok {
		return nil, c.typeError(v, "string")
	}
	return value.String(htmlUnescape(string(s))), nil
}

var charRef = regexp.MustCompile(`&(#[0-9]+;?|#[xX][0-9a-fA-F]+;?|[^\t\n\f <&#;]{1,32};?)`)

// htmlUnescape replaces named and numeric character references following
// the HTML standard rules for invalid references.
func htmlUnescape(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	return charRef.ReplaceAllStringFunc(s, func(m string) string {
		ref := m[1:]
		if ref[0] != '#' {
			return html.UnescapeString(m)
		}
		digits, base := strings.TrimSuffix(ref[1:], ";"), 10
		if digits[0] == 'x' || digits[0] == 'X' {
			digits, base = digits[1:], 16
		}
		n, err := strconv.ParseUint(digits, base, 32)
		if err != nil {
			return "�"
		}
		return numericRef(uint32(n))
	})
}

// numericRef decodes a numeric character reference.
func numericRef(n uint32) string {
	if r, ok := windows1252Ref(n); ok {
		return r
	}
	switch {
	case n >= 0xD800 && n <= 0xDFFF, n > 0x10FFFF:
		return "�"
	case n >= 0x1 && n <= 0x8, n == 0xB, n >= 0xE && n <= 0x1F, n >= 0x7F && n <= 0x9F,
		n >= 0xFDD0 && n <= 0xFDEF, n&0xFFFE == 0xFFFE:
		return ""
	}
	return string(rune(n))
}

// windows1252Ref maps the references the HTML standard replaces: NUL, CR
// and the C1 range read as windows-1252.
func windows1252Ref(n uint32) (string, bool) {
	switch n {
	case 0x00:
		return "�", true
	case 0x0D:
		return "\r", true
	}
	if n < 0x80 || n > 0x9F {
		return "", false
	}
	table := [32]rune{
		0x20AC, 0x81, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021, 0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0x8D, 0x017D, 0x8F,
		0x90, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014, 0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0x9D, 0x017E, 0x0178,
	}
	return string(table[n-0x80]), true
}
