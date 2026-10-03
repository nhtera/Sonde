// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jar

import (
	"os"
	"runtime"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/redactcheck"
	"github.com/nhtera/sonde/desktop/internal/sandboxtest"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/sandbox"
)

func TestKeepListDeleteClear(t *testing.T) {
	cfg := sandboxtest.Open(t, t.TempDir())
	proj := sandboxtest.Open(t, t.TempDir())
	keep := false
	j := New(cfg, func() *sandbox.Root { return proj }, func() bool { return keep })
	cookies := []engine.Cookie{
		{Domain: "api.example", Path: "/", Name: "sid", Value: "session-value-1", HTTPOnly: true},
		{Domain: "api.example", Path: "/", Name: "theme", Value: "dark with space", Expires: 4102444800},
	}
	j.Keep("a.hurl", cookies)
	if j.KeptJar("a.hurl") != "" {
		t.Fatal("kept a jar with keep cookies off")
	}
	keep = true
	j.Keep("a.hurl", cookies)
	j.Keep("b/c.hurl", cookies[:1])
	p := j.KeptJar("a.hurl")
	if p == "" {
		t.Fatal("no kept jar")
	}
	data, err := os.ReadFile(p)
	if err != nil || !contains(string(data), "session-value-1") {
		t.Fatalf("the kept jar holds the values (it is the run's): %q %v", data, err)
	}
	if fi, _ := os.Stat(p); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("jar mode %v", fi.Mode().Perm())
	}
	if got := parse(data); len(got) != 2 || !got[0].HTTPOnly || got[1].Value != "dark with space" || got[1].Expires != 4102444800 {
		t.Errorf("parse %+v", got)
	}
	jars, err := j.List()
	if err != nil || len(jars) != 2 || jars[0].File != "a.hurl" || len(jars[0].Cookies) != 2 {
		t.Fatalf("list %+v %v", jars, err)
	}
	redactcheck.AssertNoSecret(t, "jar list", jars, "session-value-1", "dark with space")
	if err := j.Delete("a.hurl", "api.example", "/", "sid"); err != nil {
		t.Fatal(err)
	}
	if jars, _ := j.List(); len(jars[0].Cookies) != 1 || jars[0].Cookies[0].Name != "theme" {
		t.Errorf("after delete %+v", jars[0])
	}
	if err := j.Clear(""); err != nil {
		t.Fatal(err)
	}
	if jars, _ := j.List(); len(jars) != 0 {
		t.Errorf("after clear %+v", jars)
	}
}

// Set adds a cookie (making the jar), then changes it in place; its
// value reaches the jar file only, never the list.
func TestSet(t *testing.T) {
	cfg := sandboxtest.Open(t, t.TempDir())
	proj := sandboxtest.Open(t, t.TempDir())
	j := New(cfg, func() *sandbox.Root { return proj }, func() bool { return true })
	if err := j.Set("a.hurl", SetCookie{Domain: "api.example", Name: "cart", Value: "c-first", HTTPOnly: true}); err != nil {
		t.Fatal(err)
	}
	if err := j.Set("a.hurl", SetCookie{Domain: "api.example", Path: "/", Name: "cart", Value: "c-second", Expires: 4102444800}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(j.KeptJar("a.hurl"))
	got := parse(data)
	if len(got) != 1 || got[0].Value != "c-second" || got[0].Path != "/" || got[0].Expires != 4102444800 || got[0].HTTPOnly {
		t.Fatalf("jar %+v", got)
	}
	jars, _ := j.List()
	redactcheck.AssertNoSecret(t, "jar list", jars, "c-first", "c-second")
	// A file name with a line break would write a line of its own.
	if err := j.Set("a.hurl\nevil", SetCookie{Domain: "api.example", Name: "x"}); err == nil {
		t.Error("a file name with a line break was taken")
	}
	// A kept cookie that reaches subdomains still does after an edit.
	j.Keep("b.hurl", []engine.Cookie{{Domain: "shop.example", IncludeSubdomain: true, Path: "/", Name: "sid", Value: "v1"}})
	if err := j.Set("b.hurl", SetCookie{Domain: "shop.example", Path: "/", Name: "sid", Value: "v2"}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(j.KeptJar("b.hurl"))
	if got := parse(data); len(got) != 1 || !got[0].IncludeSubdomain || got[0].Value != "v2" {
		t.Errorf("after the edit %+v", got)
	}
	for _, bad := range []SetCookie{
		{Domain: "api.example", Name: ""},
		{Domain: "", Name: "x"},
		{Domain: "api.example", Name: "x", Value: "a\tb"},
		{Domain: "api.example", Name: "x", Value: "a\nb"},
		{Domain: "api.example", Name: "a=b"},
		{Domain: "api.example", Name: "#HttpOnly_x"},
		{Domain: "api.example", Name: "x", Path: "relative"},
		{Domain: "#HttpOnly_api.example", Name: "x"},
		{Domain: "api.example", Name: "x", Value: "a; injected=1"},
		{Domain: "api.example", Name: "x", Value: "a\x00b"},
	} {
		if err := j.Set("a.hurl", bad); err == nil {
			t.Errorf("set %+v: no error", bad)
		}
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
