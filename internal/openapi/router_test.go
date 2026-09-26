// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"reflect"
	"testing"
)

func TestRouter(t *testing.T) {
	r := NewRouter([]string{
		"/pets", "/pets/{petId}", "/pets/mine", "/pets/{petId}/photos/{photoId}",
		"/orgs/{org}/repos/{repo}", "/files/{name}.json", "/files/{path}", "/",
	}, []string{"https://api.example.com/v1", "http://localhost:3000/", "/v1/beta"})
	for _, tc := range []struct {
		url, template string
		params        map[string]string
	}{
		{"https://api.example.com/v1/pets", "/pets", nil},
		{"http://127.0.0.1:9/v1/pets/", "/pets", nil},
		{"http://x/v1/pets/42", "/pets/{petId}", map[string]string{"petId": "42"}},
		{"http://x/v1/pets/mine", "/pets/mine", nil},
		{"http://x/v1/pets/a%2Fb", "/pets/{petId}", map[string]string{"petId": "a/b"}},
		{"http://x/v1/pets/1/photos/2?x=1", "/pets/{petId}/photos/{photoId}", map[string]string{"petId": "1", "photoId": "2"}},
		{"http://x/v1/orgs/acme/repos/sonde", "/orgs/{org}/repos/{repo}", map[string]string{"org": "acme", "repo": "sonde"}},
		{"http://x/v1/files/a.json", "/files/{name}.json", map[string]string{"name": "a"}},
		{"http://x/v1/files/a.txt", "/files/{path}", map[string]string{"path": "a.txt"}},
		// The root server matches paths outside /v1.
		{"http://x/pets/7", "/pets/{petId}", map[string]string{"petId": "7"}},
		{"http://x/v1", "/", nil},
		// The longer base wins when it leads to a template.
		{"http://x/v1/beta/pets", "/pets", nil},
	} {
		got, ok := r.Match(tc.url, nil)
		if !ok || got.Template != tc.template || !reflect.DeepEqual(got.Params, tc.params) {
			t.Errorf("Match(%s) = %+v, %v; want %s %v", tc.url, got, ok, tc.template, tc.params)
		}
	}
	for _, u := range []string{"http://x/v1/pets/1/2", "http://x/v1/unknown"} {
		if got, ok := r.Match(u, nil); ok {
			t.Errorf("Match(%s) = %+v, want no match", u, got)
		}
	}
}

func TestRouterBaseRequired(t *testing.T) {
	r := NewRouter([]string{"/pets"}, []string{"https://api.example.com/v1"})
	if _, ok := r.Match("http://x/pets", nil); ok {
		t.Error("a path outside the server base path matched")
	}
	if _, ok := r.Match("http://x/v1pets", nil); ok {
		t.Error("a base path matched inside a segment")
	}
	if _, ok := NewRouter([]string{"/pets"}, nil).Match("http://x/pets", nil); !ok {
		t.Error("no servers: /pets did not match")
	}
}

func TestRouterAccept(t *testing.T) {
	r := NewRouter([]string{"/pets/mine", "/pets/{id}"}, nil)
	got, ok := r.Match("http://x/pets/mine", func(t string) bool { return t == "/pets/{id}" })
	if !ok || got.Template != "/pets/{id}" || got.Params["id"] != "mine" {
		t.Errorf("Match with accept = %+v, %v", got, ok)
	}
	if _, ok := r.Match("http://x/pets/mine", func(string) bool { return false }); ok {
		t.Error("a rejected template matched")
	}
}
