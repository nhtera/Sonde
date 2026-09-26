// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"
)

func frame(body string) string {
	return "Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body
}

func TestConnMalformed(t *testing.T) {
	in := frame(`{not json`) + frame(`{"jsonrpc":"1.0","id":1,"method":"x"}`) + frame(`{"jsonrpc":"2.0","id":2,"method":"shutdown"}`)
	var out bytes.Buffer
	s, err := NewServer(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Run(context.Background(), strings.NewReader(in), &out); err != nil {
		t.Fatalf("Run = %v", err)
	}
	got := out.String()
	for _, want := range []string{`"code":-32700`, `"code":-32600`, `"id":2,"error":{"code":-32002`} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %s:\n%s", want, got)
		}
	}
}

func TestConnBadFrames(t *testing.T) {
	for _, in := range []string{
		"Content-Length: nope\r\n\r\n{}",
		"Content-Length: 999999999999\r\n\r\n{}",
		"Content-Length: 10\r\n\r\n{}",
	} {
		s, _ := NewServer(Options{})
		if err := s.Run(context.Background(), strings.NewReader(in), &bytes.Buffer{}); err == nil {
			t.Errorf("Run(%q) = nil, want error", in)
		}
	}
}

func TestConnErrorIDs(t *testing.T) {
	in := frame(`{"jsonrpc":"1.0","id":7,"method":"x"}`) +
		frame(`{"jsonrpc":"2.0","id":{"a":1},"method":"shutdown"}`) +
		frame(`{oops`)
	var out bytes.Buffer
	s, _ := NewServer(Options{})
	if err := s.Run(context.Background(), strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		`{"jsonrpc":"2.0","id":7,"error":{"code":-32600`,
		`{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"id must be a number or a string"`,
		`{"jsonrpc":"2.0","id":null,"error":{"code":-32700`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %s:\n%s", want, got)
		}
	}
	if s.shutdown {
		t.Error("a request with an object id ran")
	}
}

func TestConnHeaderLimit(t *testing.T) {
	in := "X-Pad: " + strings.Repeat("a", maxHeader) + "\r\nContent-Length: 2\r\n\r\n{}"
	s, _ := NewServer(Options{})
	if err := s.Run(context.Background(), strings.NewReader(in), &bytes.Buffer{}); err == nil {
		t.Error("oversized header accepted")
	}
}
