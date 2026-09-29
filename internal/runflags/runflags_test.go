// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runflags

import (
	"reflect"
	"testing"

	"github.com/nhtera/sonde/internal/runplan"
)

func TestArgs(t *testing.T) {
	inv := &runplan.Invocation{
		Cmd: "test", Files: []string{"a.hurl", "-x.hurl"},
		Variables: []string{"host=x"}, Proxy: "-p:1",
		MaxTime: "5", Retry: "2", Insecure: false, FromEntry: 3,
		Set: map[string]bool{"max-time": true, "insecure": true},
	}
	want := []string{"test", "--variable", "host=x", "--insecure=false",
		"--max-time", "5", "--proxy=-p:1", "--from-entry", "3", "--", "a.hurl", "-x.hurl"}
	if got := Args(inv); !reflect.DeepEqual(got, want) {
		t.Errorf("Args =\n%q\nwant\n%q", got, want)
	}
}

func TestShell(t *testing.T) {
	inv := &runplan.Invocation{
		Files:   []string{"my file.hurl"},
		Secrets: []string{"api-key=s3cr3t", "t=it's"},
		User:    "bob:pw", Set: map[string]bool{"user": true},
		Header: []string{`X-A: "q" $x`},
	}
	for _, tc := range []struct {
		dialect Dialect
		reveal  bool
		want    string
	}{
		{POSIX, false, "# Set SECRET_USER_PASSWORD, SECRET_api_key, SECRET_t first: secret values are not shown.\n" +
			`sonde run --secret api-key="$SECRET_api_key" --secret t="$SECRET_t" --user bob:"$SECRET_USER_PASSWORD" --header 'X-A: "q" $x' 'my file.hurl'`},
		{POSIX, true, `sonde run --secret api-key=s3cr3t --secret 't=it'\''s' --user bob:pw --header 'X-A: "q" $x' 'my file.hurl'`},
		{PowerShell, false, "# Set SECRET_USER_PASSWORD, SECRET_api_key, SECRET_t first: secret values are not shown.\n" +
			`sonde run --secret "api-key=$env:SECRET_api_key" --secret "t=$env:SECRET_t" --user "bob:$env:SECRET_USER_PASSWORD" --header 'X-A: "q" $x' 'my file.hurl'`},
		{Cmd, false, "REM Set SECRET_USER_PASSWORD, SECRET_api_key, SECRET_t first: secret values are not shown.\n" +
			`sonde run --secret "api-key=%SECRET_api_key%" --secret "t=%SECRET_t%" --user "bob:%SECRET_USER_PASSWORD%" --header "X-A: \"q\" $x" "my file.hurl"`},
	} {
		if got := Shell(inv, tc.dialect, tc.reveal); got != tc.want {
			t.Errorf("dialect %d reveal %v:\n%s\nwant\n%s", tc.dialect, tc.reveal, got, tc.want)
		}
	}
	joined := &runplan.Invocation{Secrets: []string{"-k=v"}}
	if got, want := Shell(joined, POSIX, false), "# Set SECRET__k first: secret values are not shown.\nsonde run --secret=-k=\"$SECRET__k\""; got != want {
		t.Errorf("joined secret:\n%s\nwant\n%s", got, want)
	}
}
