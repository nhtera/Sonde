// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"strings"
	"testing"
)

func TestShellQuoting(t *testing.T) {
	for in, want := range map[string]string{
		"certs/ca.pem":     "certs/ca.pem",
		"host:443:1.2.3.4": "host:443:1.2.3.4",
		"a b":              "'a b'",
		"$(id)":            "'$(id)'",
		"x';id;'":          `$'x\';id;\''`,
		"":                 "''",
	} {
		if got := shellArg(in); got != want {
			t.Errorf("shellArg(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestCurlCommand(t *testing.T) {
	res, _ := run(t, "POST {{base}}/echo?a=1\nX-Test: v\n[Options]\nproxy: http://p';id;'\n`body`\n", Options{})
	if len(res.Entries) == 0 {
		t.Fatal("no entry")
	}
	curl := res.Entries[0].Curl
	for _, want := range []string{"curl ", "--header 'X-Test: v'", "--data 'body'", `--proxy $'http://p\';id;\''`} {
		if !strings.Contains(curl, want) {
			t.Errorf("curl command %q lacks %q", curl, want)
		}
	}
}
