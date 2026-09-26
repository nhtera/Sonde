// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nhtera/sonde/exchange"
)

func TestBuildURLQuery(t *testing.T) {
	cases := []struct {
		url, want string
	}{
		{"http://h/p", "http://h/p?a=1"},
		{"http://h/p?x=1", "http://h/p?x=1&a=1"},
		{"http://h/p?", "http://h/p?a=1"},
	}
	for _, tt := range cases {
		u, err := buildURL(&RequestSpec{URL: tt.url, Query: []Param{{Name: "a", Value: "1"}}}, false)
		if err != nil {
			t.Fatalf("%s: %v", tt.url, err)
		}
		if u.String() != tt.want {
			t.Errorf("buildURL(%s) = %s, want %s", tt.url, u.String(), tt.want)
		}
	}
}

func TestBuildURLQueryEscaping(t *testing.T) {
	u, err := buildURL(&RequestSpec{URL: "http://h/p", Query: []Param{{Name: "a b", Value: "c d&e"}}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if u.RawQuery != "a b=c%20d%26e" {
		t.Errorf("RawQuery = %q", u.RawQuery)
	}
}

func TestBuildURLInvalidScheme(t *testing.T) {
	_, err := buildURL(&RequestSpec{URL: "ftp://h/p"}, false)
	var herr *Error
	if !asError(err, &herr) || herr.Kind != ErrInvalidURL {
		t.Fatalf("err = %v", err)
	}
}

func TestPathAsIs(t *testing.T) {
	u, err := buildURL(&RequestSpec{URL: "http://h/a/../b"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/b" {
		t.Errorf("normalized path = %q, want /b", u.Path)
	}
	u2, err := buildURL(&RequestSpec{URL: "http://h/a/../b"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if u2.Path != "/a/../b" {
		t.Errorf("path-as-is path = %q, want /a/../b", u2.Path)
	}
}

func TestCurlEscape(t *testing.T) {
	if got := curlEscape("a b/c~d_e.f-g"); got != "a%20b%2Fc~d_e.f-g" {
		t.Errorf("curlEscape = %q", got)
	}
}

func TestBuildBodyForm(t *testing.T) {
	b, err := buildBody(&RequestSpec{Form: []Param{{Name: "a", Value: "1 2"}, {Name: "b", Value: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if b.contentType != "application/x-www-form-urlencoded" {
		t.Errorf("contentType = %q", b.contentType)
	}
	if string(b.data) != "a=1%202&b=x" {
		t.Errorf("data = %q", b.data)
	}
}

func TestBuildBodyMultipart(t *testing.T) {
	spec := &RequestSpec{Multipart: []MultipartParam{
		{Param: &Param{Name: "field", Value: "value"}},
		{File: &FileParam{Name: "file", Filename: "a.txt", Data: []byte("content"), ContentType: "text/plain"}},
	}}
	b, err := buildBody(spec)
	if err != nil {
		t.Fatal(err)
	}
	mediaType, params, err := mime.ParseMediaType(b.contentType)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/form-data") {
		t.Fatalf("contentType = %q, err = %v", b.contentType, err)
	}
	mr := multipart.NewReader(strings.NewReader(string(b.data)), params["boundary"])
	var fields, files int
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(part)
		if part.FormName() == "field" {
			fields++
			if string(data) != "value" {
				t.Errorf("field value = %q", data)
			}
		}
		if part.FormName() == "file" {
			files++
			if part.FileName() != "a.txt" || string(data) != "content" {
				t.Errorf("file part = %q %q", part.FileName(), data)
			}
			if ct := part.Header.Get("Content-Type"); ct != "text/plain" {
				t.Errorf("file content-type = %q", ct)
			}
		}
	}
	if fields != 1 || files != 1 {
		t.Errorf("fields=%d files=%d", fields, files)
	}
}

func TestImplicitContentTypeNotOverridden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	spec := &RequestSpec{Method: "POST", URL: srv.URL, Body: Body{Kind: BodyText, Data: []byte(`{"a":1}`)}, ImplicitContentType: "application/json"}
	_, err := c.Execute(context.Background(), spec, &Options{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestExplicitContentTypeWins(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "text/custom" {
			t.Errorf("Content-Type = %q", ct)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	spec := &RequestSpec{
		Method:              "POST",
		URL:                 srv.URL,
		Body:                Body{Kind: BodyText, Data: []byte("x")},
		ImplicitContentType: "application/json",
		Headers:             []exchange.Header{{Name: "Content-Type", Value: "text/custom"}},
	}
	_, err := c.Execute(context.Background(), spec, &Options{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestUserBasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "alice" || p != "secret" {
			t.Errorf("BasicAuth = %q %q %v", u, p, ok)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	get(t, c, srv.URL, &Options{User: "alice:secret"})
}

func TestCompressedAcceptEncoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") != "gzip, deflate, br" {
			t.Errorf("Accept-Encoding = %q", r.Header.Get("Accept-Encoding"))
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	get(t, c, srv.URL, &Options{Compressed: true})
}

func TestExtraOptionHeadersAppended(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Opt") != "v" {
			t.Errorf("X-Opt = %q", r.Header.Get("X-Opt"))
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	get(t, c, srv.URL, &Options{Headers: []exchange.Header{{Name: "X-Opt", Value: "v"}}})
}

// Credentials in the URL become Basic authentication and are not sent in
// the request line.
func TestURLUserinfo(t *testing.T) {
	var auth, uri string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		auth, uri = r.Header.Get("Authorization"), r.RequestURI
	}))
	defer srv.Close()
	c := newTestClient(t, ClientConfig{})
	u := strings.Replace(srv.URL, "http://", "http://bob%40email.com:secret@", 1) + "/p"
	calls, err := c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: u}, &Options{})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Basic Ym9iQGVtYWlsLmNvbTpzZWNyZXQ=" || uri != "/p" {
		t.Errorf("auth = %q, uri = %q", auth, uri)
	}
	names := []string{}
	for _, h := range calls[0].Request.Headers {
		names = append(names, h.Name)
	}
	if strings.Join(names, ",") != "Host,Authorization,Accept,User-Agent" {
		t.Errorf("headers = %v", names)
	}
}
