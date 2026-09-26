// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package postman

import "testing"

func TestKebabName(t *testing.T) {
	for in, want := range map[string]string{
		"Local Dev": "local-dev", "PROD": "prod", "  spaced  ": "spaced",
		"a.b_c": "a-b-c", "": "",
	} {
		if got := kebabName(in); got != want {
			t.Errorf("kebabName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUniqueEnvName(t *testing.T) {
	used := map[string]bool{"collection": true}
	got := []string{
		uniqueEnvName("Prod", used),
		uniqueEnvName("prod", used),
		uniqueEnvName("prod", used),
		uniqueEnvName("", used),
	}
	want := []string{"prod", "prod-2", "prod-3", "environment"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("name %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseEnvironmentFallbackName(t *testing.T) {
	w := newWalker(0)
	name, vars, secrets, err := w.parseEnvironment(EnvironmentFile{
		FileName: "/tmp/staging.postman_environment.json",
		Data:     []byte(`{"values":[{"key":"host","value":"h"},{"key":"pw","value":"x","type":"secret"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "staging.postman_environment" {
		t.Errorf("fallback name = %q", name)
	}
	if vars["host"] != "h" {
		t.Errorf("vars = %v", vars)
	}
	if len(secrets) != 1 || secrets[0] != "pw" {
		t.Errorf("secrets = %v", secrets)
	}
	found := false
	for _, wn := range w.out.Warnings {
		if wn.Kind == "secret" {
			found = true
		}
	}
	if !found {
		t.Error("expected a secret warning")
	}
}

func TestParseEnvironmentInvalidJSON(t *testing.T) {
	w := newWalker(0)
	if _, _, _, err := w.parseEnvironment(EnvironmentFile{FileName: "x.json", Data: []byte("{not json")}); err == nil {
		t.Error("expected an error for invalid environment JSON")
	}
}

func TestEnvironmentDisabledValueSkipped(t *testing.T) {
	w := newWalker(0)
	_, vars, _, err := w.parseEnvironment(EnvironmentFile{
		FileName: "e.json",
		Data:     []byte(`{"name":"e","values":[{"key":"on","value":"1","enabled":true},{"key":"off","value":"2","enabled":false}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := vars["off"]; ok {
		t.Error("a disabled value should be skipped")
	}
	if vars["on"] != "1" {
		t.Errorf("vars = %v", vars)
	}
}
