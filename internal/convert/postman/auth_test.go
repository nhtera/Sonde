// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package postman

import (
	"encoding/json"
	"testing"
)

// TestAuthParamListObjectShape checks the v2.0 object shape
// ("basic": {"username": "u", ...}), not just the v2.1 array of
// {key, value}, unmarshals into the same authParam list.
func TestAuthParamListObjectShape(t *testing.T) {
	var l authParamList
	if err := json.Unmarshal([]byte(`{"username": "u", "password": "p"}`), &l); err != nil {
		t.Fatal(err)
	}
	if paramValue(l, "username") != "u" || paramValue(l, "password") != "p" {
		t.Errorf("params = %+v", l)
	}
}

// TestRequestStringShape checks the bare-URL string shape both schema
// versions allow for "request" ({"request": "https://..."} is the same as
// {"request": {"method": "GET", "url": "https://..."}}).
func TestRequestStringShape(t *testing.T) {
	var r request
	if err := json.Unmarshal([]byte(`"https://api.example.test/ping"`), &r); err != nil {
		t.Fatal(err)
	}
	if r.Method != "GET" {
		t.Errorf("method = %q, want GET", r.Method)
	}
	if string(r.URL) != `"https://api.example.test/ping"` {
		t.Errorf("url = %s", r.URL)
	}
}

// TestHeaderListStringShape checks the raw "Key: value\nKey2: value2"
// string shape the schema also allows for "header".
func TestHeaderListStringShape(t *testing.T) {
	var h headerList
	if err := json.Unmarshal([]byte(`"X-Trace: abc\nAccept: application/json"`), &h); err != nil {
		t.Fatal(err)
	}
	if len(h) != 2 || h[0].Key != "X-Trace" || string(h[0].Value) != "abc" || h[1].Key != "Accept" {
		t.Errorf("headers = %+v", h)
	}
}

func TestExtractStatus(t *testing.T) {
	for _, tc := range []struct{ code, want string }{
		{"pm.response.to.have.status(200);", "200"},
		{"pm.response.to.have.status(201, \"Created\");", "201"},
		{"pm.expect(pm.response.code).to.eql(204);", "204"},
		{"pm.expect(pm.response.code).to.equal(404);", "404"},
		{"pm.test('ok', function () { pm.response.to.be.ok; });", ""},
		{"", ""},
	} {
		if got := extractStatus(tc.code); got != tc.want {
			t.Errorf("extractStatus(%q) = %q, want %q", tc.code, got, tc.want)
		}
	}
}

func TestAuthStateInheritanceAndNoauth(t *testing.T) {
	bearer := &auth{Type: "bearer", Bearer: []authParam{{Key: "token", Value: "t"}}}
	root := authState{}.resolve(bearer)
	if root.auth != bearer {
		t.Fatal("resolve with own set should override")
	}
	// An absent own auth field inherits the parent's.
	child := root.resolve(nil)
	if child.auth != bearer {
		t.Error("nil own auth should inherit the parent's")
	}
	// noauth is an explicit override that stops inheritance.
	none := root.resolve(&auth{Type: "noauth"})
	if none.auth == nil || none.auth.Type != "noauth" {
		t.Error("noauth should be recorded as an explicit override")
	}
	grandchild := none.resolve(nil)
	if grandchild.auth.Type != "noauth" {
		t.Error("noauth should keep propagating to descendants that set nothing of their own")
	}
}

func TestParamValue(t *testing.T) {
	params := []authParam{{Key: "username", Value: "u"}, {Key: "password", Value: "p"}}
	if v := paramValue(params, "username"); v != "u" {
		t.Errorf("username = %q", v)
	}
	if v := paramValue(params, "missing"); v != "" {
		t.Errorf("missing = %q, want empty", v)
	}
}
