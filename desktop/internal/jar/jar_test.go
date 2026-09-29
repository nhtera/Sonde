// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jar

import (
	"os"
	"runtime"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/redactcheck"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/sandbox"
)

func TestKeepListDeleteClear(t *testing.T) {
	cfg, _ := sandbox.Open(t.TempDir())
	proj, _ := sandbox.Open(t.TempDir())
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

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
