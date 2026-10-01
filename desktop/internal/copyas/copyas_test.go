// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package copyas

import (
	"context"
	"strings"
	"testing"

	"github.com/nhtera/sonde/desktop/internal/redactcheck"
	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/runplan"
)

const (
	token = "copy-token-sentinel-4" //nolint:gosec // G101: test sentinel
	proxy = "proxy-pw-sentinel-9"   //nolint:gosec // G101: test sentinel
)

type planner struct{}

func (planner) Planned(context.Context, string, string, string, bool) (engine.Options, error) {
	return engine.Options{
		Variables: map[string]any{"base": "https://api.example"},
		Secrets:   map[string]string{"tok": token},
	}, nil
}

func (planner) Invocation(cmd, env, data, kind string, files []string) runplan.Invocation {
	inv := runplan.Invocation{Cmd: cmd, Env: env, FileRoot: ".", Files: files, Set: map[string]bool{"file-root": true, "env": env != ""}}
	inv.Proxy, inv.Set["proxy"] = "http://u:"+proxy+"@proxy.example:3128", true
	inv.Variables, inv.Set["variable"] = []string{"user_id=7"}, true
	_ = data
	_ = kind
	return inv
}

const src = "GET {{base}}/me\nAuthorization: Bearer {{tok}}\nHTTP 200\n\nGET {{base}}/orders/{{order}}\nHTTP 200\n\nGET wss://x/ws\n[SondeMessages]\nsend: `hi`\n"

func copier() *Copier { return New(planner{}, func() string { return "/p" }) }

func TestCurl(t *testing.T) {
	c := copier()
	txt, err := c.Curl(context.Background(), Request{File: "a.sonde", Source: src}, false)
	if err != nil {
		t.Fatal(err)
	}
	redactcheck.AssertNoSecret(t, "curl", txt, token)
	if n := strings.Count(txt.Text, "curl "); n != 2 {
		t.Errorf("%d curl commands:\n%s", n, txt.Text)
	}
	if !strings.Contains(txt.Note, "order") || !strings.Contains(txt.Note, "Request 3 has no curl command") {
		t.Errorf("note %q", txt.Note)
	}
	one, err := c.Curl(context.Background(), Request{File: "a.sonde", Source: src, Entry: 2}, false)
	if err != nil || strings.Count(one.Text, "curl ") != 1 || !strings.Contains(one.Text, "/orders/") {
		t.Errorf("entry 2: %+v %v", one, err)
	}
	var clip string
	r := NewReveal(c, func(s string) error { clip = s; return nil })
	note, err := r.Curl(context.Background(), Request{File: "a.sonde", Source: src, Entry: 1})
	if err != nil || !strings.Contains(clip, token) {
		t.Errorf("reveal: %q %v", clip, err)
	}
	if strings.Contains(note, token) {
		t.Error("the note reaches the page: it must not hold the secret")
	}
}

func TestSonde(t *testing.T) {
	c := copier()
	run, err := c.Sonde(Request{File: "api/a.hurl", Env: "local"}, false)
	if err != nil {
		t.Fatal(err)
	}
	redactcheck.AssertNoSecret(t, "sonde", run, proxy)
	for _, want := range []string{"sonde run", "--env local", "--file-root .", "--variable user_id=7", "api/a.hurl"} {
		if !strings.Contains(run.Text, want) {
			t.Errorf("run command lacks %q:\n%s", want, run.Text)
		}
	}
	if !strings.Contains(run.Note, "/p") {
		t.Errorf("note %q", run.Note)
	}
	send, err := c.Sonde(Request{File: "a.hurl", Kind: "send", Entry: 3}, false)
	if err != nil || !strings.Contains(send.Text, "--to-entry 3") || !strings.Contains(send.Note, "1–3") {
		t.Errorf("send %+v %v", send, err)
	}
	test, err := c.Sonde(Request{Kind: "test", Files: []string{"a.hurl", "b.hurl"}, Shell: "powershell"}, false)
	if err != nil || !strings.HasPrefix(strings.SplitN(test.Text, "\n", 2)[len(strings.SplitN(test.Text, "\n", 2))-1], "sonde test") || !strings.Contains(test.Text, "b.hurl") {
		t.Errorf("test %+v %v", test, err)
	}
	var clip string
	r := NewReveal(c, func(s string) error { clip = s; return nil })
	if _, err := r.Sonde(Request{File: "a.hurl"}); err != nil || !strings.Contains(clip, proxy) {
		t.Errorf("reveal %q %v", clip, err)
	}
	if _, err := c.Sonde(Request{File: "a.hurl", Kind: "nope"}, false); err == nil {
		t.Error("unknown kind")
	}
	if _, err := c.Sonde(Request{File: "a%b.hurl", Shell: "cmd"}, false); err == nil {
		t.Error("cmd.exe cannot hold %")
	}
}

// TestSondeCommandVariables: the command adds what Command adds, and the
// note names the variables it holds back.
func TestSondeCommandVariables(t *testing.T) {
	c := copier()
	c.Command = func(inv *runplan.Invocation) []string {
		inv.Variables = append(inv.Variables, "region=eu")
		return []string{"api_token"}
	}
	run, err := c.Sonde(Request{File: "a.hurl"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(run.Text, "--variable region=eu") || !strings.Contains(run.Note, "Set SONDE_VARIABLE_api_token there too") {
		t.Errorf("%+v", run)
	}
}
