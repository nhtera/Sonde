// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nhtera/sonde/exchange"
)

// TestAuthTokensRedacted checks that the credentials an authentication
// scheme derives (a Digest response, an NTLM Type-3 message, an AWS
// signature) are recorded masked, so neither the verbose logs nor the
// reports (built from the recorded calls) show them.
func TestAuthTokensRedacted(t *testing.T) {
	challenge := "TlRMTVNTUAACAAAAAwAMADgAAAAzgoriASNFZ4mrze8AAAAAAAAAACQAJABEAAAABgBwFwAAAA9TAGUAcgB2AGUAcgACAAwARABvAG0AYQBpAG4AAQAMAFMAZQByAHYAZQByAAAAAAA="
	var sent string // the last Authorization the server got
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		sent = auth
		switch {
		case strings.HasPrefix(auth, "Digest "), strings.HasPrefix(auth, "AWS4-HMAC-SHA256 "):
			_, _ = io.WriteString(w, "ok")
		case strings.HasPrefix(auth, "NTLM "):
			msg, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "NTLM "))
			if len(msg) > 8 && msg[8] == 3 {
				_, _ = io.WriteString(w, "ok")
				return
			}
			w.Header().Set("WWW-Authenticate", "NTLM "+challenge)
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/digest":
			w.Header().Set("WWW-Authenticate", `Digest realm="r", nonce="n", qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()
	for name, option := range map[string]string{
		"digest":    "digest: true",
		"ntlm":      "ntlm: true",
		"aws-sigv4": "aws-sigv4: aws:amz:eu-west-1:svc",
	} {
		t.Run(name, func(t *testing.T) {
			src := "GET " + srv.URL + "/" + name + "\n[Options]\n" + option + "\nuser: user:pw\nHTTP 200\n"
			rec := &recorder{}
			r := NewRunner(Options{Verbosity: Verbose, OnEvent: rec.on})
			res, err := r.RunSource(t.Context(), filepath.Join(t.TempDir(), "t.hurl"), []byte(src))
			if err != nil {
				t.Fatal(err)
			}
			if errs := res.Errors(); len(errs) != 0 {
				t.Fatalf("errors %v", errs)
			}
			call := res.Entries[0].Calls[0]
			if recorded, _ := exchange.Headers(call.Request.Headers).Get("Authorization"); recorded != "***" {
				t.Errorf("recorded Authorization is not masked")
			}
			token := sent
			if token == "" {
				t.Fatal("no Authorization sent")
			}
			var logs strings.Builder
			for _, l := range rec.logs {
				logs.WriteString(l.Text + "\n")
			}
			if strings.Contains(logs.String(), token) {
				t.Errorf("the token shows in the logs")
			}
			if !strings.Contains(logs.String(), "Authorization: ***") {
				t.Errorf("no masked Authorization line in the logs")
			}
		})
	}
}
