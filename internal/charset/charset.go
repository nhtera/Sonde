// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package charset looks up text encodings by their WHATWG labels and decodes
// bytes strictly.
package charset

import (
	"bytes"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
)

// Lookup returns the encoding for a label such as "utf-8", "latin1" or
// "Shift_JIS" (case-insensitive, surrounding white space ignored).
func Lookup(label string) (encoding.Encoding, bool) {
	enc, err := htmlindex.Get(label)
	return enc, err == nil
}

// Decode decodes b strictly: malformed or unmappable input fails.
// A byte order mark is decoded as a character, not stripped.
func Decode(enc encoding.Encoding, b []byte) (string, bool) {
	switch name, _ := htmlindex.Name(enc); name {
	case "utf-8":
		if !utf8.Valid(b) {
			return "", false
		}
		return string(b), true
	case "utf-16le", "utf-16be":
		return decodeUTF16(b, name == "utf-16be")
	case "replacement":
		return "", len(b) == 0
	}
	out, err := enc.NewDecoder().Bytes(b)
	if err != nil || bytes.ContainsRune(out, utf8.RuneError) {
		return "", false
	}
	return string(out), true
}

func decodeUTF16(b []byte, bigEndian bool) (string, bool) {
	if len(b)%2 != 0 {
		return "", false
	}
	units := make([]uint16, len(b)/2)
	for i := range units {
		if bigEndian {
			units[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
		} else {
			units[i] = uint16(b[2*i+1])<<8 | uint16(b[2*i])
		}
	}
	var sb strings.Builder
	for i := 0; i < len(units); i++ {
		u := rune(units[i])
		switch {
		case utf16.IsSurrogate(u) && u < 0xDC00 && i+1 < len(units):
			r := utf16.DecodeRune(u, rune(units[i+1]))
			if r == utf8.RuneError {
				return "", false
			}
			sb.WriteRune(r)
			i++
		case utf16.IsSurrogate(u):
			return "", false
		default:
			sb.WriteRune(u)
		}
	}
	return sb.String(), true
}

// Name returns the standard name of an encoding, e.g. "UTF-8",
// "ISO-8859-2" or "windows-1252".
func Name(enc encoding.Encoding) string {
	name, _ := htmlindex.Name(enc)
	switch name {
	case "utf-8", "ibm866", "koi8-r", "koi8-u", "gbk", "euc-jp", "iso-2022-jp", "euc-kr", "utf-16be", "utf-16le":
		return strings.ToUpper(name)
	case "big5":
		return "Big5"
	case "shift_jis":
		return "Shift_JIS"
	}
	if strings.HasPrefix(name, "iso-8859-") {
		return strings.ToUpper(name)
	}
	return name
}
