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
	inv := &runplan.Invocation{ //nolint:gosec // G101: test fixture
		Files:   []string{"my file.hurl"},
		Secrets: []string{"api-key=s3cr3t", "t=it's"},
		User:    "bob:pw", Set: map[string]bool{"user": true},
		Header: []string{`X-A: "q" $x & y`, "Authorization: Bearer abc"},
		Cert:   `C:\certs\me.pem:certpw`,
		Proxy:  "http://pu:ppw@proxy.test:3128",
	}
	note := "Set SECRET_CERT_PASSWORD, SECRET_HEADER_AUTHORIZATION, SECRET_PROXY_PASSWORD, SECRET_USER_PASSWORD, SECRET_api_key, SECRET_t first: secret values are not shown.\n"
	for _, tc := range []struct {
		dialect Dialect
		reveal  bool
		want    string
	}{
		{POSIX, false, "# " + note + `sonde run --secret api-key="${SECRET_api_key}" --secret t="${SECRET_t}" --cert 'C:\certs\me.pem:'"${SECRET_CERT_PASSWORD}" ` +
			`--proxy http://pu:"${SECRET_PROXY_PASSWORD}"@proxy.test:3128 --user bob:"${SECRET_USER_PASSWORD}" --header 'X-A: "q" $x & y' ` +
			`--header 'Authorization: '"${SECRET_HEADER_AUTHORIZATION}" 'my file.hurl'`},
		{POSIX, true, `sonde run --secret api-key=s3cr3t --secret 't=it'\''s' --cert 'C:\certs\me.pem:certpw' --proxy http://pu:ppw@proxy.test:3128 ` +
			`--user bob:pw --header 'X-A: "q" $x & y' --header 'Authorization: Bearer abc' 'my file.hurl'`},
		{PowerShell, false, "# " + note + `sonde run --secret "api-key=${env:SECRET_api_key}" --secret "t=${env:SECRET_t}" --cert "C:\certs\me.pem:${env:SECRET_CERT_PASSWORD}" ` +
			`--proxy "http://pu:${env:SECRET_PROXY_PASSWORD}@proxy.test:3128" --user "bob:${env:SECRET_USER_PASSWORD}" --header 'X-A: "q" $x & y' ` +
			`--header "Authorization: ${env:SECRET_HEADER_AUTHORIZATION}" 'my file.hurl'`},
		{Cmd, false, "REM " + note + `sonde run --secret "api-key=%SECRET_api_key%" --secret "t=%SECRET_t%" --cert "C:\certs\me.pem:%SECRET_CERT_PASSWORD%" ` +
			`--proxy "http://pu:%SECRET_PROXY_PASSWORD%@proxy.test:3128" --user "bob:%SECRET_USER_PASSWORD%" --header "X-A: ""q"" $x & y" ` +
			`--header "Authorization: %SECRET_HEADER_AUTHORIZATION%" "my file.hurl"`},
	} {
		got, err := Shell(inv, tc.dialect, tc.reveal)
		if err != nil || got != tc.want {
			t.Errorf("dialect %d reveal %v (%v):\n%s\nwant\n%s", tc.dialect, tc.reveal, err, got, tc.want)
		}
	}
	joined := &runplan.Invocation{Secrets: []string{"-k=v"}}
	if got, _ := Shell(joined, POSIX, false); got != "# Set SECRET__k first: secret values are not shown.\nsonde run --secret=-k=\"${SECRET__k}\"" {
		t.Errorf("joined secret: %s", got)
	}
	// cmd.exe can not hold a line break, % or !; PowerShell doubles
	// typographic single quotes.
	for _, v := range []string{"a\nb", "100%", "hi!"} {
		if _, err := Shell(&runplan.Invocation{Header: []string{"X: " + v}}, Cmd, false); err == nil {
			t.Errorf("cmd accepted %q", v)
		}
	}
	if got, _ := Shell(&runplan.Invocation{Header: []string{"X: a\u2019b"}}, PowerShell, false); got != "sonde run --header 'X: a\u2019\u2019b'" {
		t.Errorf("PowerShell smart quote: %s", got)
	}
}
