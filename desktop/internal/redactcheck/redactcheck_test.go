// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package redactcheck

import (
	"encoding/base64"
	"net/url"
	"testing"
)

func TestFind(t *testing.T) {
	const secret = "s3cr/t+v@lue" //nolint:gosec // G101: test sentinel
	for _, in := range []string{
		"x " + secret,
		base64.StdEncoding.EncodeToString([]byte(secret)),
		base64.RawURLEncoding.EncodeToString([]byte(secret)),
		"q=" + url.QueryEscape(secret),
	} {
		if Find([]byte(in), secret) == "" {
			t.Errorf("missed %q", in)
		}
	}
	if got := Find([]byte("nothing here ***"), secret); got != "" {
		t.Errorf("false positive: %s", got)
	}
	if got := Find([]byte(secret), secret); got == "" || contains(got, secret) {
		t.Errorf("the report must not print the secret: %q", got)
	}
}

func TestAssertNoSecret(t *testing.T) {
	ft := &testing.T{}
	AssertNoSecret(ft, "dto", map[string]string{"a": "***"}, "token-123")
	if ft.Failed() {
		t.Error("clean value failed")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
