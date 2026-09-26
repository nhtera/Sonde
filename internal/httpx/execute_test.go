// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nhtera/sonde/exchange"
)

func get(t *testing.T, c *Client, url string, opts *Options) []Call {
	t.Helper()
	if opts == nil {
		opts = &Options{}
	}
	calls, err := c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: url}, opts)
	if err != nil {
		t.Fatalf("Execute(%s): %v", url, err)
	}
	return calls
}

func TestExecuteBasicGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/hi" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	calls := get(t, c, srv.URL+"/hi", nil)
	if len(calls) != 1 {
		t.Fatalf("calls = %d", len(calls))
	}
	if string(calls[0].Response.Body) != "ok" {
		t.Errorf("body = %q", calls[0].Response.Body)
	}
	if calls[0].Response.Status != 200 {
		t.Errorf("status = %d", calls[0].Response.Status)
	}
}

func TestExecuteHeadersQueryUserAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("b"); got != "hello world" {
			t.Errorf("query b = %q, want %q", got, "hello world")
		}
		if r.Header.Get("X-Test") != "v" {
			t.Errorf("X-Test = %q", r.Header.Get("X-Test"))
		}
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "sonde/") {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("Accept") != "*/*" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	spec := &RequestSpec{
		Method:  "GET",
		URL:     srv.URL + "/",
		Query:   []Param{{Name: "a", Value: "1"}, {Name: "b", Value: "hello world"}},
		Headers: []exchange.Header{{Name: "X-Test", Value: "v"}},
	}
	_, err := c.Execute(context.Background(), spec, &Options{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestExecuteRedirect(t *testing.T) {
	var mux http.ServeMux
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/end", http.StatusFound)
	})
	mux.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("redirected method = %s", r.Method)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte("done"))
	})
	srv := httptest.NewServer(&mux)
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	calls := get(t, c, srv.URL+"/start", &Options{FollowLocation: true})
	if len(calls) != 2 {
		t.Fatalf("calls = %d", len(calls))
	}
	if calls[1].Response.Status != 200 || string(calls[1].Response.Body) != "done" {
		t.Errorf("final call = %+v", calls[1].Response)
	}
}

func TestExecuteRedirectMethodChange(t *testing.T) {
	statuses := []int{301, 302, 303, 307, 308}
	for _, status := range statuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var seenMethod, seenBody string
			var mux http.ServeMux
			mux.HandleFunc("/start", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", "/end")
				w.WriteHeader(status)
			})
			mux.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
				seenMethod = r.Method
				b := make([]byte, 64)
				n, _ := r.Body.Read(b)
				seenBody = string(b[:n])
				w.WriteHeader(200)
			})
			srv := httptest.NewServer(&mux)
			defer srv.Close()

			c := newTestClient(t, ClientConfig{})
			spec := &RequestSpec{Method: "POST", URL: srv.URL + "/start", Body: Body{Kind: BodyText, Data: []byte("payload")}}
			_, err := c.Execute(context.Background(), spec, &Options{FollowLocation: true})
			if err != nil {
				t.Fatal(err)
			}
			wantMethod := "POST"
			if status >= 301 && status <= 303 {
				wantMethod = "GET"
			}
			if seenMethod != wantMethod {
				t.Errorf("status %d: method = %s, want %s", status, seenMethod, wantMethod)
			}
			if wantMethod == "GET" && seenBody != "" {
				t.Errorf("status %d: body kept on GET redirect: %q", status, seenBody)
			}
			if wantMethod == "POST" && seenBody != "payload" {
				t.Errorf("status %d: body dropped: %q", status, seenBody)
			}
		})
	}
}

func TestExecuteRedirectAbsoluteAndRelative(t *testing.T) {
	var mux http.ServeMux
	var srv *httptest.Server
	mux.HandleFunc("/rel", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "end")
		w.WriteHeader(302)
	})
	mux.HandleFunc("/abs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", srv.URL+"/end")
		w.WriteHeader(302)
	})
	mux.HandleFunc("/end", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})
	srv = httptest.NewServer(&mux)
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	for _, path := range []string{"/rel", "/abs"} {
		calls := get(t, c, srv.URL+path, &Options{FollowLocation: true})
		if len(calls) != 2 || calls[1].Response.Status != 200 {
			t.Errorf("%s: calls = %+v", path, calls)
		}
	}
}

func TestExecuteMaxRedirects(t *testing.T) {
	var mux http.ServeMux
	mux.HandleFunc("/loop", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/loop")
		w.WriteHeader(302)
	})
	srv := httptest.NewServer(&mux)
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	_, err := c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: srv.URL + "/loop"},
		&Options{FollowLocation: true, MaxRedirects: 3})
	if err == nil {
		t.Fatal("expected too many redirects error")
	}
	var herr *Error
	if !asError(err, &herr) || herr.Kind != ErrTooManyRedirects {
		t.Fatalf("err = %v", err)
	}
}

func TestExecuteCredentialDropOnHostChange(t *testing.T) {
	var mux http.ServeMux
	var seenAuth, seenCookie string
	var otherSrv *httptest.Server
	mux.HandleFunc("/start", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", otherSrv.URL+"/end")
		w.WriteHeader(302)
	})
	srv := httptest.NewServer(&mux)
	defer srv.Close()

	var mux2 http.ServeMux
	mux2.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenCookie = r.Header.Get("Cookie")
		w.WriteHeader(200)
	})
	otherSrv = httptest.NewServer(&mux2)
	defer otherSrv.Close()

	c := newTestClient(t, ClientConfig{})
	for name, opts := range map[string]*Options{
		"user":           {FollowLocation: true, User: "alice:secret"},
		"option headers": {FollowLocation: true, Headers: []exchange.Header{{Name: "authorization", Value: "Bearer t"}, {Name: "Cookie", Value: "c=1"}}},
	} {
		seenAuth, seenCookie = "", ""
		spec := &RequestSpec{Method: "GET", URL: srv.URL + "/start",
			Headers: []exchange.Header{{Name: "Authorization", Value: "Bearer e"}},
			Cookies: []RequestCookie{{Name: "s", Value: "2"}}}
		if _, err := c.Execute(context.Background(), spec, opts); err != nil {
			t.Fatal(err)
		}
		if seenAuth != "" || seenCookie != "s=2" {
			t.Errorf("%s: other host got Authorization %d bytes, Cookie %q; want none and the entry cookie only", name, len(seenAuth), seenCookie)
		}
	}
}

func TestExecuteCredentialKeptOnLocationTrusted(t *testing.T) {
	var mux http.ServeMux
	var seenAuth string
	var otherSrv *httptest.Server
	mux.HandleFunc("/start", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", otherSrv.URL+"/end")
		w.WriteHeader(302)
	})
	srv := httptest.NewServer(&mux)
	defer srv.Close()

	var mux2 http.ServeMux
	mux2.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	})
	otherSrv = httptest.NewServer(&mux2)
	defer otherSrv.Close()

	c := newTestClient(t, ClientConfig{})
	spec := &RequestSpec{Method: "GET", URL: srv.URL + "/start"}
	opts := &Options{FollowLocation: true, User: "alice:secret", LocationTrusted: true}
	_, err := c.Execute(context.Background(), spec, opts)
	if err != nil {
		t.Fatal(err)
	}
	if seenAuth == "" {
		t.Error("Authorization not forwarded with location-trusted")
	}
}

func TestExecuteCredentialKeptOnSameHost(t *testing.T) {
	var mux http.ServeMux
	var seenAuth string
	mux.HandleFunc("/start", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/end")
		w.WriteHeader(302)
	})
	mux.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	})
	srv := httptest.NewServer(&mux)
	defer srv.Close()

	c := newTestClient(t, ClientConfig{})
	spec := &RequestSpec{Method: "GET", URL: srv.URL + "/start"}
	opts := &Options{FollowLocation: true, User: "alice:secret"}
	_, err := c.Execute(context.Background(), spec, opts)
	if err != nil {
		t.Fatal(err)
	}
	if seenAuth == "" {
		t.Error("Authorization dropped on same-host redirect")
	}
}

func TestExecuteUnsupportedOptions(t *testing.T) {
	c := newTestClient(t, ClientConfig{})
	tests := []struct {
		name string
		opts Options
	}{
		{"aws-sigv4", Options{AWSSigV4: "aws:amz:us-east-1:s3"}},
		{"digest", Options{Digest: true}},
		{"ntlm", Options{NTLM: true}},
		{"negotiate", Options{Negotiate: true}},
		{"http1.0", Options{HTTPVersion: HTTP10}},
		{"http3", Options{HTTPVersion: HTTP3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.Execute(context.Background(), &RequestSpec{Method: "GET", URL: "http://example.invalid/"}, &tt.opts)
			var herr *Error
			if !asError(err, &herr) || herr.Kind != ErrUnsupported {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// asError is a small errors.As wrapper kept local to avoid an extra import
// line in every test that needs it.
func asError(err error, target **Error) bool {
	he, ok := err.(*Error)
	if ok {
		*target = he
	}
	return ok
}
