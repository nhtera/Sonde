// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"strings"
	"testing"
)

const sondeStreams = `# SSE
GET {{base_url}}/events
[Options]
sonde-stream-count: 3
sonde-stream-timeout: 5s
sonde-stream-max-bytes: {{limit}}
HTTP 200
[Asserts]
sondeStream count == 3
sondeStream "event" nth 0 == "ready"
sondeStream nth 1 jsonpath "$.progress" == 50

GET ws://{{host}}/ws
Authorization: Bearer {{token}}
[SondeMessages]
send: {"type": "ping", "color": "#fff"}  # a comment
send: ` + "`hello`" + `
send: hex,00ff10;
send: {
  "multi": "line"
}
send: ` + "```" + `
raw text
` + "```" + `
receive
  receive:   2
close: 1001
close
HTTP 101
[Captures]
first: sondeStream nth 0 jsonpath "$.type"
[Asserts]
sondeStream "type" nth 2 == "binary"
`

func TestSondeConstructsParse(t *testing.T) {
	f, err := Parse("s.sonde", []byte(sondeStreams), DialectSonde)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(Print(f)); got != sondeStreams {
		t.Errorf("Print is not lossless:\n%s", got)
	}
	opts := f.Entries[0].Request.Sections[0].Options
	if len(opts) != 3 || opts[0].Name != "sonde-stream-count" || opts[1].Value.(*Duration).Unit != "s" {
		t.Errorf("options = %+v", opts)
	}
	asserts := f.Entries[0].Response.Sections[0].Asserts
	if q := asserts[0].Query; q.Kind != QuerySondeStream || q.Arg != nil || len(asserts[0].Filters) != 1 {
		t.Errorf("sondeStream count: %+v", q)
	}
	if q := asserts[1].Query; q.Arg.(*StreamField).Name != "event" {
		t.Errorf("field = %+v", q.Arg)
	}

	s := f.Entries[1].Request.Sections[0]
	if s.Kind != SectionMessages || s.Name != "SondeMessages" {
		t.Fatalf("section = %v %q", s.Kind, s.Name)
	}
	type step struct {
		kind  StepKind
		colon bool
		value string
	}
	want := []step{
		{StepSend, true, `{"type": "ping", "color": "#fff"}`},
		{StepSend, true, "`hello`"},
		{StepSend, true, "hex,00ff10;"},
		{StepSend, true, "{\n  \"multi\": \"line\"\n}"},
		{StepSend, true, "```\nraw text\n```"},
		{StepReceive, false, ""},
		{StepReceive, true, "2"},
		{StepClose, true, "1001"},
		{StepClose, false, ""},
	}
	if len(s.Messages) != len(want) {
		t.Fatalf("%d steps, want %d", len(s.Messages), len(want))
	}
	for i, m := range s.Messages {
		var p printer
		p.node(m.Value)
		if got := (step{m.Kind, m.Colon, p.String()}); got != want[i] {
			t.Errorf("step %d = %+v, want %+v", i, got, want[i])
		}
	}
	if c := s.Messages[0].LineTerminator0.Comment; c == nil || c.Value != " a comment" {
		t.Errorf("comment = %+v", c)
	}
}

func TestSondeFormat(t *testing.T) {
	src := "GET ws://h/ws\n[SondeMessages]\n  send :{\"a\":1}   \nreceive:3\n  close\nHTTP 101\n[Asserts]\nsondeStream   \"data\"   count == 1\n"
	want := "GET ws://h/ws\n[SondeMessages]\nsend: {\"a\":1}\nreceive: 3\nclose\nHTTP 101\n[Asserts]\nsondeStream \"data\" count == 1\n"
	f, err := Parse("s.sonde", []byte(src), DialectSonde)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(Format(f)); got != want {
		t.Errorf("Format:\n%s\nwant:\n%s", got, want)
	}
}

// TestSondeBodyAfterMessages checks a body is still a body after the steps.
func TestSondeBodyAfterMessages(t *testing.T) {
	for _, body := range []string{"true", "null", "hex,00;", "base64,AA==;", `{"a": 1}`} {
		src := "GET ws://h\n[SondeMessages]\nreceive\n" + body + "\n"
		f, err := Parse("s.sonde", []byte(src), DialectSonde)
		if err != nil {
			t.Errorf("%s: %v", body, err)
			continue
		}
		if r := f.Entries[0].Request; r.Body == nil || len(r.Sections[0].Messages) != 1 {
			t.Errorf("%s: body = %v, steps = %d", body, r.Body, len(r.Sections[0].Messages))
		}
	}
}

func TestSondeErrors(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		d         Dialect
		kind      ErrorKind
		line, col int
		msg       string
	}{
		{"section in hurl", "GET http://h\n[SondeMessages]\nreceive\n", DialectHurl, ErrSondeOnly, 2, 2,
			"section `[SondeMessages]` requires a .sonde file"},
		{"option in hurl", "GET http://h\n[Options]\nsonde-stream-count: 3\n", DialectHurl, ErrSondeOnly, 3, 1,
			"option `sonde-stream-count` requires a .sonde file"},
		{"query in hurl", "GET http://h\nHTTP 200\n[Asserts]\nsondeStream count == 1\n", DialectHurl, ErrSondeOnly, 4, 1,
			"query `sondeStream` requires a .sonde file"},
		{"section typo in hurl", "GET http://h\n[SondeMessage]\n", DialectHurl, ErrRequestSectionName, 2, 2,
			"the section is not valid. Valid values are Query, Form, Multipart, Cookies, Options"},
		{"section typo in sonde", "GET http://h\n[SondeMessage]\n", DialectSonde, ErrRequestSectionName, 2, 2,
			"the section is not valid. Did you mean SondeMessages?"},
		{"option typo in hurl", "GET http://h\n[Options]\nsonde-stream-cont: 3\n", DialectHurl, ErrInvalidOption, 3, 1,
			"the option name is not valid. Valid values are aws-sigv4,"},
		{"option typo in sonde", "GET http://h\n[Options]\nsonde-stream-cont: 3\n", DialectSonde, ErrInvalidOption, 3, 1,
			"the option name is not valid. Did you mean sonde-stream-count?"},
		{"messages in response", "GET ws://h\nHTTP 101\n[SondeMessages]\n", DialectSonde, ErrResponseSectionName, 3, 2,
			"the section is not valid."},
		{"step typo", "GET ws://h\n[SondeMessages]\nrecive\n", DialectSonde, ErrMessageStep, 3, 1,
			"the step is not valid. Did you mean receive?"},
		{"unknown step", "GET ws://h\n[SondeMessages]\nping: 1\n", DialectSonde, ErrMessageStep, 3, 1,
			"Valid values are send, receive, close"},
		{"send without value", "GET ws://h\n[SondeMessages]\nsend\n", DialectSonde, ErrExpecting, 3, 5, "expecting ':'"},
		{"send plain word", "GET ws://h\n[SondeMessages]\nsend: hello\n", DialectSonde, ErrMessageValue, 3, 7, "expecting a message"},
		{"receive not a number", "GET ws://h\n[SondeMessages]\nreceive: x\n", DialectSonde, ErrExpecting, 3, 10, "integer >= 1"},
		{"receive zero", "GET ws://h\n[SondeMessages]\nreceive: 0\n", DialectSonde, ErrExpecting, 3, 10, "integer >= 1"},
		{"reserved close code", "GET ws://h\n[SondeMessages]\nclose: 1005\n", DialectSonde, ErrExpecting, 3, 8, "close code 1000-4999"},
		{"close code too large", "GET ws://h\n[SondeMessages]\nclose: 99999\n", DialectSonde, ErrExpecting, 3, 8, "close code 1000-4999"},
		{"step without colon", "GET ws://h\n[SondeMessages]\nreceive 2\n", DialectSonde, ErrExpecting, 3, 8, "expecting ':'"},
		{"step typo without colon", "GET ws://h\n[SondeMessages]\nrecieve 2\n", DialectSonde, ErrMessageStep, 3, 1, "the step is not valid"},
		{"bad field", "GET http://h\nHTTP 200\n[Asserts]\nsondeStream \"evnt\" count == 1\n", DialectSonde, ErrStreamField, 4, 14,
			"the field is not valid. Did you mean event?"},
		{"stream option value", "GET http://h\n[Options]\nsonde-stream-timeout: 5x\n", DialectSonde, ErrInvalidDurationUnit, 3, 24, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse("f", []byte(tc.src), tc.d)
			var pe *Error
			if !errors.As(err, &pe) {
				t.Fatalf("err = %v, want a parse error", err)
			}
			if pe.Kind != tc.kind || pe.Pos.Line != tc.line || pe.Pos.Col != tc.col || !strings.Contains(pe.Message(), tc.msg) {
				t.Errorf("got %v %d:%d %q, want %v %d:%d %q", pe.Kind, pe.Pos.Line, pe.Pos.Col, pe.Message(),
					tc.kind, tc.line, tc.col, tc.msg)
			}
		})
	}
}

// TestSondeParseAllGate checks the recovering parser applies the same gate.
func TestSondeParseAllGate(t *testing.T) {
	src := []byte("GET http://h\n[SondeMessages]\nreceive\n\nGET http://h\nHTTP 200\n[Asserts]\nsondeStream count == 1\n")
	if _, errs := ParseAll("f.hurl", src, DialectHurl, 0); len(errs) != 2 || errs[0].Kind != ErrSondeOnly || errs[1].Kind != ErrSondeOnly {
		t.Errorf("hurl errs = %v", errs)
	}
	if _, errs := ParseAll("f.sonde", src, DialectSonde, 0); len(errs) != 0 {
		t.Errorf("sonde errs = %v", errs)
	}
}
