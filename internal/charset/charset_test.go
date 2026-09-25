// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package charset

import "testing"

func TestName(t *testing.T) {
	for label, want := range map[string]string{
		"utf8": "UTF-8", "latin1": "windows-1252", "latin2": "ISO-8859-2", "sjis": "Shift_JIS",
		"big5": "Big5", "gbk": "GBK", "koi8-u": "KOI8-U", "macintosh": "macintosh", "gb18030": "gb18030",
	} {
		enc, ok := Lookup(label)
		if !ok {
			t.Fatalf("Lookup(%s) failed", label)
		}
		if got := Name(enc); got != want {
			t.Errorf("Name(%s) = %q, want %q", label, got, want)
		}
	}
	if _, ok := Lookup("nope"); ok {
		t.Error("Lookup(nope) succeeded")
	}
}

func TestDecode(t *testing.T) {
	tests := []struct {
		label string
		in    []byte
		want  string
		ok    bool
	}{
		{"utf-8", []byte("café"), "café", true},
		{"utf-8", []byte("caf\xe9"), "", false},
		{"latin1", []byte("caf\xe9"), "café", true},
		{"utf-16le", []byte{'h', 0, 0x3d, 0xd8, 0x00, 0xde}, "h😀", true},
		{"utf-16be", []byte{0, 'h', 0xff, 0xfd}, "h�", true},
		{"utf-16be", []byte{0, 'h', 0}, "", false},
		{"utf-16le", []byte{0x00, 0xdc}, "", false},
		{"utf-16le", []byte{0x3d, 0xd8, 'a', 0}, "", false},
		{"utf-16le", []byte{0x3d, 0xd8}, "", false},
		{"shift_jis", []byte("\x82\xa0"), "あ", true},
		{"shift_jis", []byte("\x82"), "", false},
		{"windows-1253", []byte("\xaa"), "", false},
		{"replacement", nil, "", true},
		{"replacement", []byte("a"), "", false},
	}
	for _, tt := range tests {
		enc, _ := Lookup(tt.label)
		got, ok := Decode(enc, tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Decode(%s, %q) = %q, %v", tt.label, tt.in, got, ok)
		}
	}
}
