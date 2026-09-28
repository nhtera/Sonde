// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package grpcx

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/nhtera/sonde/exchange"
)

func TestStatus(t *testing.T) {
	h := func(kv ...[2]string) exchange.Headers {
		var out exchange.Headers
		for _, p := range kv {
			out = append(out, exchange.Header{Name: p[0], Value: p[1]})
		}
		return out
	}
	for _, tc := range []struct {
		name   string
		http   int
		h      exchange.Headers
		code   int
		status string
		msg    string
	}{
		{"ok", 200, h([2]string{"grpc-status", "0"}), 0, "OK", ""},
		{"message", 200, h([2]string{"Grpc-Status", "5"}, [2]string{"grpc-message", "not%20found: 100%25 %zz%"}), 5, "NOT_FOUND", "not found: 100% %zz%"},
		{"unknown code", 200, h([2]string{"grpc-status", "42"}), 42, "UNKNOWN", ""},
		{"invalid code", 200, h([2]string{"grpc-status", "x"}), 2, "UNKNOWN", `invalid grpc-status "x"`},
		{"http 503", 503, nil, 14, "UNAVAILABLE", "HTTP status 503 without grpc-status"},
		{"http 404", 404, nil, 12, "UNIMPLEMENTED", "HTTP status 404 without grpc-status"},
		{"http 418", 418, nil, 2, "UNKNOWN", "HTTP status 418 without grpc-status"},
		{"no status", 200, h([2]string{"content-type", "application/grpc"}), 13, "INTERNAL", "the server ended the call without grpc-status"},
		{"not grpc", 200, h([2]string{"content-type", "text/html"}), 2, "UNKNOWN", `the response is not gRPC (content type "text/html")`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := Status(tc.http, tc.h)
			if st.Code != tc.code || st.Status != tc.status || st.Message != tc.msg {
				t.Errorf("status = %+v", st)
			}
		})
	}
}

func TestResetStatus(t *testing.T) {
	for text, want := range map[string]string{
		"stream error: stream ID 1; CANCEL; received from peer":         "CANCELLED",
		"stream error: stream ID 3; REFUSED_STREAM":                     "UNAVAILABLE",
		"http2: stream error: stream ID 1; INTERNAL_ERROR; from peer":   "INTERNAL",
		"stream error: stream ID 1; ENHANCE_YOUR_CALM; received from p": "RESOURCE_EXHAUSTED",
	} {
		st, ok := ResetStatus(errors.New(text))
		if !ok || st.Status != want {
			t.Errorf("%q: %+v %v, want %s", text, st, ok, want)
		}
	}
	if _, ok := ResetStatus(errors.New("connection refused")); ok {
		t.Error("not a reset")
	}
}

func TestTimeout(t *testing.T) {
	for d, want := range map[time.Duration]string{
		5 * time.Second:           "5S",
		1500 * time.Millisecond:   "1500m",
		300 * time.Second:         "5M",
		time.Nanosecond:           "1n",
		1234567 * time.Nanosecond: "1234567n",
		200 * time.Hour:           "200H",
	} {
		if got := Timeout(d); got != want {
			t.Errorf("Timeout(%v) = %s, want %s", d, got, want)
		}
	}
	if got := Timeout(1<<63 - 1); len(got) > 9 {
		t.Errorf("Timeout(max) = %s", got)
	}
}

func TestParseTimeout(t *testing.T) {
	for v, want := range map[string]time.Duration{"5S": 5 * time.Second, "200m": 200 * time.Millisecond, "1n": 1, "99999999H": 1<<63 - 1} {
		if d, ok := ParseTimeout(v); !ok || d != want {
			t.Errorf("ParseTimeout(%q) = %v %v", v, d, ok)
		}
	}
	for _, v := range []string{"", "5", "S", "+5S", "-0S", "5s", "123456789S", " 5S"} {
		if _, ok := ParseTimeout(v); ok {
			t.Errorf("ParseTimeout(%q) accepted", v)
		}
	}
}

func TestIsGRPC(t *testing.T) {
	for ct, want := range map[string]bool{"application/grpc": true, "application/grpc+proto": true, "Application/GRPC": true,
		"application/grpc-web": false, "text/html": false, "": false} {
		if got := IsGRPC(exchange.Headers{{Name: "Content-Type", Value: ct}}); got != want {
			t.Errorf("IsGRPC(%q) = %v", ct, got)
		}
	}
	if !IsGRPC(exchange.Headers{{Name: "grpc-status", Value: "0"}}) {
		t.Error("a status without content type")
	}
}

func TestParser(t *testing.T) {
	p, _ := NewParser("")
	stream := append(Frame([]byte("one")), Frame([]byte("two"))...)
	var got []string
	for i := range stream { // one byte at a time
		msgs, err := p.Write(stream[i : i+1])
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range msgs {
			got = append(got, string(m))
		}
	}
	if strings.Join(got, ",") != "one,two" || p.Close() != nil {
		t.Errorf("messages = %v", got)
	}

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte("zipped"))
	_ = zw.Close()
	frame := Frame(buf.Bytes())
	frame[0] = 1
	gz, _ := NewParser("gzip")
	if msgs, err := gz.Write(frame); err != nil || len(msgs) != 1 || string(msgs[0]) != "zipped" {
		t.Errorf("gzip: %q %v", msgs, err)
	}
	if _, err := p.Write(frame); err == nil || !strings.Contains(err.Error(), "without grpc-encoding") {
		t.Errorf("compressed without encoding: %v", err)
	}
	if _, err := NewParser("snappy"); err == nil {
		t.Error("snappy accepted")
	}
	big, _ := NewParser("")
	if _, err := big.Write([]byte{0, 0x7f, 0, 0, 0}); err == nil || !strings.Contains(err.Error(), "more than the limit") {
		t.Errorf("large message: %v", err)
	}
	cut, _ := NewParser("")
	_, _ = cut.Write(Frame([]byte("abc"))[:6])
	if cut.Close() == nil {
		t.Error("a cut message is not an error")
	}
}

func TestSplitPath(t *testing.T) {
	if s, m, err := SplitPath("/a.b.Svc/Do"); err != nil || s != "a.b.Svc" || m != "Do" {
		t.Errorf("= %s %s %v", s, m, err)
	}
	for _, p := range []string{"", "/", "/Svc", "/Svc/", "//Do", "/Svc/Do/x", "Svc/Do"} {
		if _, _, err := SplitPath(p); err == nil {
			t.Errorf("%q accepted", p)
		}
	}
}

func TestReflectErrors(t *testing.T) {
	errorReply := func(code uint64, msg string) []byte {
		var e []byte
		e = protowire.AppendTag(e, 1, protowire.VarintType)
		e = protowire.AppendVarint(e, code)
		e = protowire.AppendTag(e, 2, protowire.BytesType)
		e = protowire.AppendString(e, msg)
		r := protowire.AppendTag(nil, 7, protowire.BytesType)
		return protowire.AppendBytes(r, e)
	}
	var paths []string
	invoke := func(_ context.Context, path string, _ []byte) ([]byte, error) {
		paths = append(paths, path)
		return errorReply(5, "symbol not found"), nil
	}
	_, err := Reflect(context.Background(), "a.Svc", invoke)
	if err == nil || !strings.Contains(err.Error(), "server reflection (grpc.reflection.v1): gRPC status NOT_FOUND: symbol not found") || len(paths) != 1 {
		t.Errorf("err = %v, paths = %v", err, paths)
	}
	_, err = Reflect(context.Background(), "a.Svc", func(context.Context, string, []byte) ([]byte, error) {
		return []byte{0xff}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "invalid reflection reply") {
		t.Errorf("err = %v", err)
	}
}
