// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntaxedit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/nhtera/sonde/engine"
)

func TestURLQueryModel(t *testing.T) {
	for _, tc := range []struct {
		name, url string
		want      []URLParam
		kept      string
	}{
		{"none", "http://h/p", nil, ""},
		{"bare question mark", "http://h/p?", nil, ""},
		// A curl import keeps the URL as it was copied.
		{"decoded", "http://h/p?from=2026-09-06T15%3A00%3A00.000Z&to=x", []URLParam{{"from", "2026-09-06T15:00:00.000Z"}, {"to", "x"}}, ""},
		{"plus is a space", "http://h/p?q=a+b%20c", []URLParam{{"q", "a b c"}}, ""},
		{"placeholder in the base", "{{base}}/p?q=x%2Fy", []URLParam{{"q", "x/y"}}, ""},
		// A variable's value is sent as is in the URL, encoded in [Query].
		{"placeholder in a value", "{{base}}/p?a=1&id={{id}}", []URLParam{{"a", "1"}, {"id", "{{id}}"}}, `"{{id}}" has a {{variable}}, sent as is in the URL`},
		{"semicolon", "http://h/p?a=1;b=2", []URLParam{{"a", "1;b=2"}}, `"1;b=2" has a ;`},
		{"escaped for the row", "http://h/p?tag=%23x&b=%7B%7Bx%7D%7D&c%3A=1", []URLParam{{"tag", `\#x`}, {"b", `\u{7B}{x}}`}, {`c\u{25}3A`, "1"}}, ""},
		{"empty pieces skipped", "http://h/p?a=1&&b=&", []URLParam{{"a", "1"}, {"b", ""}}, ""},
		{"flag", "http://h/p?a=1&flag", []URLParam{{"a", "1"}, {"flag", ""}}, `"flag" is not name=value`},
		{"whole query in a variable", "http://h/p?{{qs}}", []URLParam{{"{{qs}}", ""}}, `"{{qs}}" is not name=value`},
		{"bad escape", "http://h/p?a=100%", []URLParam{{"a", "100%"}}, `"100%" has a % that is not an escape`},
		{"not text", "http://h/p?a=%FF", []URLParam{{"a", "%FF"}}, `"%FF" decodes to bytes that are not text`},
		{"fragment", `http://h/p?a=1\#top`, []URLParam{{"a", `1\#top`}}, "the URL has a #fragment"},
		{"fragment before the query", `http://h/p\#f?x=1`, []URLParam{{"x", "1"}}, "the URL has a #fragment"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ms, err := Model("t.hurl", []byte("GET "+tc.url+"\n"))
			if err != nil {
				t.Fatal(err)
			}
			m := ms[0]
			if !reflect.DeepEqual(m.URLQuery, tc.want) {
				t.Errorf("URLQuery = %q, want %q", m.URLQuery, tc.want)
			}
			if m.URLQueryKept != tc.kept {
				t.Errorf("URLQueryKept = %q, want %q", m.URLQueryKept, tc.kept)
			}
		})
	}
}

func TestMoveURLQuery(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"new section",
			"GET http://h/api?from=2026-09-06T15%3A00%3A00.000Z&to=2026-10-06T15%3A00%3A00.000Z\nAccept: */*\n[Cookies]\nmode: auto-dark\n",
			"GET http://h/api\nAccept: */*\n[Cookies]\nmode: auto-dark\n[Query]\nfrom: 2026-09-06T15:00:00.000Z\nto: 2026-10-06T15:00:00.000Z\n",
		},
		{
			// Sent first: written before the section's rows, disabled
			// ones included.
			"before the rows",
			"GET http://h/p?x=1 # note\n[Query]\n# page: 2\nlang: é\n",
			"GET http://h/p # note\n[Query]\nx: 1\n# page: 2\nlang: é\n",
		},
		{"empty section", "GET http://h/p?x=1\n[Query]\n[Options]\ninsecure: true\n", "GET http://h/p\n[Query]\nx: 1\n[Options]\ninsecure: true\n"},
		{"before the body", "POST http://h/p?x=1\n{\"a\": 1}\n", "POST http://h/p\n[Query]\nx: 1\n{\"a\": 1}\n"},
		{"no final newline", "GET http://h/p?x=1", "GET http://h/p\n[Query]\nx: 1\n"},
		// The base is kept as written.
		{"placeholders", "GET {{ base }}/p\\u{41}?tag=%23a\n", "GET {{ base }}/p\\u{41}\n[Query]\ntag: \\#a\n"},
		{"crlf", "GET http://h/p?x=1\r\nAccept: */*\r\n", "GET http://h/p\r\nAccept: */*\r\n[Query]\r\nx: 1\r\n"},
		// Only the new rows take the file's line ending.
		{"mixed line endings", "GET http://h/p?x=1\r\nA: 1\nB: 2\r\n", "GET http://h/p\r\nA: 1\nB: 2\r\n[Query]\r\nx: 1\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := apply(t, tc.src, func(src []byte) (*Result, error) { return MoveURLQuery("t.hurl", src, 1) })
			if got != tc.want {
				t.Errorf("got\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

func TestMoveURLQuerySecondEntry(t *testing.T) {
	src := "GET http://h/a\n\nGET http://h/b?x=1\nHTTP 200\n"
	got := apply(t, src, func(src []byte) (*Result, error) { return MoveURLQuery("t.hurl", src, 2) })
	if want := "GET http://h/a\n\nGET http://h/b\n[Query]\nx: 1\nHTTP 200\n"; got != want {
		t.Errorf("got\n%s", got)
	}
}

func TestMoveURLQueryRefused(t *testing.T) {
	for _, src := range []string{
		"GET http://h/p\n",
		"GET http://h/p?a=1&flag\n",
		"GET http://h/p?a=1\\#top\n",
		"GET http://h/p?a=%zz\n",
		"GET http://h/p?id={{id}}\n",
	} {
		if _, err := MoveURLQuery("t.hurl", []byte(src), 1); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: err = %v, want ErrInvalid", src, err)
		}
	}
}

// TestMoveURLQuerySendsTheSameRequest runs each file before and after the
// move: the request line the server reads is the same, byte for byte.
func TestMoveURLQuerySendsTheSameRequest(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r.RequestURI }))
	t.Cleanup(srv.Close)
	send := func(src string) string {
		t.Helper()
		got = ""
		res, err := engine.NewRunner(engine.Options{}).RunSource(context.Background(), "t.hurl", []byte(src))
		if err != nil || res.ParseError != nil || !res.Success {
			t.Fatalf("run: %v %v\n%s", err, res.ParseError, src)
		}
		return got
	}
	for _, query := range []string{
		"from=2026-09-06T15%3A00%3A00.000Z&to=2026-10-06T15%3A00%3A00.000Z",
		"q=a%20b&tag=%23x&b=%7B%7Bx%7D%7D&a[]=1&c%3A=%C3%A9",
	} {
		src := "GET " + srv.URL + "/api?" + query + "\n[Query]\nlang: é\n"
		moved := apply(t, src, func(src []byte) (*Result, error) { return MoveURLQuery("t.hurl", src, 1) })
		if strings.Contains(moved, "?") {
			t.Fatalf("not moved:\n%s", moved)
		}
		before, after := send(src), send(moved)
		if before != after {
			t.Errorf("sent %q, then %q\n%s", before, after, moved)
		}
	}
}
