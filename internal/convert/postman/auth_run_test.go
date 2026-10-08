// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package postman

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/syntax"
)

// TestImportedAuthRuns runs imported digest and awsv4 auth: the request
// is signed (the session token header included) or answers the challenge,
// and the secret never reaches the server.
func TestImportedAuthRuns(t *testing.T) {
	const secret = "imported-secret-value"
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		got = append(got, auth)
		if r.URL.Path == "/digest" && !strings.HasPrefix(auth, "Digest ") {
			w.Header().Set("WWW-Authenticate", `Digest realm="r", nonce="n", qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()

	collection := `{"info": {"name": "Auth", "schema": "https://schema.postman.com/json/collection/v2.1.0/collection.json"}, "item": [
	{"name": "sigv4", "request": {"method": "GET", "url": "{{base_url}}/aws", "auth": {"type": "awsv4", "awsv4": [
		{"key": "accessKey", "value": "AKIDEXAMPLE"}, {"key": "secretKey", "value": "{{secret}}"},
		{"key": "region", "value": "us-east-1"}, {"key": "service", "value": "execute-api"},
		{"key": "sessionToken", "value": "token"}]}}},
	{"name": "digest", "request": {"method": "GET", "url": "{{base_url}}/digest", "auth": {"type": "digest", "digest": [
		{"key": "username", "value": "u"}, {"key": "password", "value": "{{secret}}"}]}}}]}`
	out, err := Import([]byte(collection), Options{Group: GroupRequest, Dialect: syntax.DialectHurl})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Warnings) != 0 {
		t.Errorf("warnings %v", out.Warnings)
	}
	r := engine.NewRunner(engine.Options{Variables: map[string]any{"base_url": srv.URL, "secret": secret}})
	for _, f := range out.Files {
		res, err := r.RunSource(context.Background(), f.Path, syntax.Lint(f.File))
		if err != nil || !res.Success {
			t.Fatalf("%s: %v %+v", f.Path, err, res)
		}
	}
	if len(got) != 3 {
		t.Fatalf("requests %q", got)
	}
	if !strings.HasPrefix(got[0], "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/") || !strings.Contains(got[0], "x-amz-security-token") {
		t.Errorf("sigv4 Authorization %q", got[0])
	}
	if got[1] != "" || !strings.HasPrefix(got[2], `Digest username="u"`) {
		t.Errorf("digest Authorization %q", got[1:])
	}
	for _, a := range got {
		if strings.Contains(a, secret) || strings.HasPrefix(a, "Basic ") {
			t.Errorf("credentials sent as is: %q", a)
		}
	}
}
