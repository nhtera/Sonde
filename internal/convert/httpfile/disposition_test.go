// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpfile

import "testing"

// TestDispositionParamNonASCII: a line whose lowercase form has another
// byte length (İ grows, invalid UTF-8 becomes U+FFFD) is read by its own
// byte positions, without a panic (found by FuzzHTTPFile).
func TestDispositionParamNonASCII(t *testing.T) {
	cases := []struct{ line, key, want string }{
		{"Content-Disposition: form-data; İİİİ; name=\"a\"", "name=", "a"},
		{"Content-Disposition: form-data; \xff\xff\xff; NAME=b; x", "name=", "b"},
		{"Content-Disposition: form-data; name=\"f\"; FILENAME=\"İ.txt\"", "filename=", "İ.txt"},
		{"İİİİİİİİİİİİİİİİİİİİ name=", "name=", ""},
	}
	for _, c := range cases {
		if got := dispositionParam(c.line, c.key); got != c.want {
			t.Errorf("dispositionParam(%q, %q) = %q, want %q", c.line, c.key, got, c.want)
		}
	}
}
