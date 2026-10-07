// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runplan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
)

func set(flags ...string) map[string]bool {
	m := map[string]bool{}
	for _, f := range flags {
		m[f] = true
	}
	return m
}

func mustNew(t *testing.T, inv *Invocation, env config.Env) *Plan {
	t.Helper()
	p, err := New(inv, env, "test")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestPrecedence checks, for every setting the environment can give, that
// the environment applies without the flag and the flag wins over it.
func TestPrecedence(t *testing.T) {
	for _, tc := range []struct {
		flag, env, envVal string
		inv               Invocation
		get               func(*Plan) any
		fromEnv, fromFlag any
	}{
		{"compressed", "COMPRESSED", "true", Invocation{Compressed: false}, func(p *Plan) any { return p.Options.HTTP.Compressed }, true, false},
		{"insecure", "INSECURE", "1", Invocation{}, func(p *Plan) any { return p.Options.HTTP.Insecure }, true, false},
		{"user", "USER", "a:b", Invocation{User: "c:d"}, func(p *Plan) any { return p.Options.HTTP.User }, "a:b", "c:d"},
		{"user-agent", "USER_AGENT", "env-ua", Invocation{UserAgent: "flag-ua"}, func(p *Plan) any { return p.Options.HTTP.UserAgent }, "env-ua", "flag-ua"},
		{"location-trusted", "LOCATION_TRUSTED", "true", Invocation{}, func(p *Plan) any { return p.Options.HTTP.LocationTrusted }, true, false},
		{"connect-timeout", "CONNECT_TIMEOUT", "7", Invocation{ConnectTimeout: "3"}, func(p *Plan) any { return p.Options.HTTP.ConnectTimeout }, 7 * time.Second, 3 * time.Second},
		{"max-time", "MAX_TIME", "9", Invocation{MaxTime: "2"}, func(p *Plan) any { return p.Options.HTTP.Timeout }, 9 * time.Second, 2 * time.Second},
		{"max-redirs", "MAX_REDIRS", "4", Invocation{MaxRedirs: "5"}, func(p *Plan) any { return p.Options.HTTP.MaxRedirects }, 4, 5},
		{"max-filesize", "MAX_FILESIZE", "100", Invocation{MaxFilesize: "200"}, func(p *Plan) any { return p.Options.HTTP.MaxFilesize }, int64(100), int64(200)},
		{"limit-rate", "LIMIT_RATE", "10", Invocation{LimitRate: "20"}, func(p *Plan) any { return p.Options.HTTP.LimitRate }, int64(10), int64(20)},
		// --ipv6=false and --http2=false fall through to the environment,
		// as the CLI always did.
		{"ipv6", "IPV6", "true", Invocation{IPv6: false}, func(p *Plan) any { return p.Options.HTTP.IPResolve }, engine.IPv6, engine.IPv6},
		{"http2", "HTTP2", "true", Invocation{HTTP2: false}, func(p *Plan) any { return p.Options.HTTP.HTTPVersion }, engine.HTTP2, engine.HTTP2},
		{"http3", "HTTP3", "true", Invocation{HTTP3: false}, func(p *Plan) any { return p.Options.HTTP.HTTPVersion }, engine.HTTP3, engine.HTTP3},
		{"http1.1", "HTTP11", "true", Invocation{HTTP11: false}, func(p *Plan) any { return p.Options.HTTP.HTTPVersion }, engine.HTTP11, engine.HTTP11},
		{"ipv4", "IPV4", "true", Invocation{IPv4: false}, func(p *Plan) any { return p.Options.HTTP.IPResolve }, engine.IPv4, engine.IPv4},
		{"location", "LOCATION", "true", Invocation{Location: true}, func(p *Plan) any { return p.Options.HTTP.FollowLocation }, true, true},
		{"jobs", "JOBS", "3", Invocation{Cmd: "test", Jobs: 5}, func(p *Plan) any { return p.Workers }, 1, 5},
		{"verbosity", "VERBOSITY", "brief", Invocation{Verbosity: "debug"}, func(p *Plan) any { return p.Options.Verbosity }, engine.Brief, engine.VeryVerbose},
		{"delay", "DELAY", "50", Invocation{Delay: "20"}, func(p *Plan) any { return p.Options.Delay }, 50 * time.Millisecond, 20 * time.Millisecond},
		{"retry", "RETRY", "3", Invocation{Retry: "1"}, func(p *Plan) any { return p.Options.Retry }, 3, 1},
		{"retry-interval", "RETRY_INTERVAL", "30", Invocation{RetryInterval: "10"}, func(p *Plan) any { return p.Options.RetryInterval }, 30 * time.Millisecond, 10 * time.Millisecond},
		{"no-assert", "NO_ASSERT", "true", Invocation{}, func(p *Plan) any { return p.Options.NoAssert }, true, false},
		{"continue-on-error", "CONTINUE_ON_ERROR", "true", Invocation{}, func(p *Plan) any { return p.Options.ContinueOnError }, true, false},
		{"no-cookie-store", "NO_COOKIE_STORE", "true", Invocation{}, func(p *Plan) any { return p.Options.NoCookieStore }, true, false},
		{"repeat", "REPEAT", "3", Invocation{Repeat: "2"}, func(p *Plan) any { return p.Repeat }, 3, 2},
		{"parallel", "PARALLEL", "true", Invocation{}, func(p *Plan) any { return p.Parallel }, true, false},
		{"test", "TEST", "true", Invocation{}, func(p *Plan) any { return p.Test }, true, false},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			env := config.Env{"HURL_" + tc.env: tc.envVal}
			byEnv := mustNew(t, &Invocation{}, env)
			if got := tc.get(byEnv); got != tc.fromEnv {
				t.Errorf("from the environment: %v, want %v", got, tc.fromEnv)
			}
			if !reflect.DeepEqual(byEnv.Provenance[:1], []Source{{Setting: "--" + tc.flag, Origin: "HURL_" + tc.env}}) {
				t.Errorf("provenance %v", byEnv.Provenance)
			}
			inv := tc.inv
			inv.Set = set(tc.flag)
			byFlag := mustNew(t, &inv, env)
			if got := tc.get(byFlag); got != tc.fromFlag {
				t.Errorf("from the flag: %v, want %v", got, tc.fromFlag)
			}
			for _, s := range byFlag.Provenance {
				if s.Setting == "--"+tc.flag {
					t.Errorf("a flag given still has provenance %v", s)
				}
			}
		})
	}
}

// TestSondeEnvWins checks that SONDE_* wins over HURL_*.
func TestSondeEnvWins(t *testing.T) {
	p := mustNew(t, &Invocation{}, config.Env{"HURL_RETRY": "1", "SONDE_RETRY": "2"})
	if p.Options.Retry != 2 || p.Provenance[0].Origin != "SONDE_RETRY" {
		t.Errorf("retry %d, provenance %v", p.Options.Retry, p.Provenance)
	}
}

// TestConfigFile checks the user config file, below the environment and
// the flags.
func TestConfigFile(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "hurl")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte("--verbose\n--header X-File: 1\n--max-redirs 7\n--user-agent file-ua\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config")
	env := config.Env{"HOME": home, "HURL_HEADER": "X-Env: 2"}
	p := mustNew(t, &Invocation{Header: []string{"X-Flag: 3"}}, env)
	h := p.Options.HTTP
	if h.UserAgent != "file-ua" || h.MaxRedirects != 7 || p.Options.Verbosity != engine.Verbose {
		t.Errorf("config file values: %+v %v", h, p.Options.Verbosity)
	}
	if !reflect.DeepEqual(h.Headers, []string{"X-File: 1", "X-Env: 2", "X-Flag: 3"}) {
		t.Errorf("headers %v", h.Headers)
	}
	want := []Source{{"--header", "HURL_HEADER"}, {"--verbose", path}, {"--header", path}, {"--max-redirs", path}, {"--user-agent", path}}
	if !reflect.DeepEqual(p.Provenance, want) {
		t.Errorf("provenance\n%v\nwant\n%v", p.Provenance, want)
	}
	p = mustNew(t, &Invocation{UserAgent: "flag", MaxRedirs: "1", Verbosity: "brief", Set: set("user-agent", "max-redirs", "verbosity")}, env)
	if p.Options.HTTP.UserAgent != "flag" || p.Options.HTTP.MaxRedirects != 1 || p.Options.Verbosity != engine.Brief {
		t.Errorf("flags lose to the config file: %+v", p.Options.HTTP)
	}
	if err := os.WriteFile(path, []byte("--nope\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(&Invocation{}, env, "test"); err == nil {
		t.Error("a broken config file is not reported")
	}
}

// writeConfig writes a config file under a new $XDG_CONFIG_HOME and
// returns its path and an environment that selects it.
func writeConfig(t *testing.T, content string) (string, config.Env) {
	t.Helper()
	xdg := t.TempDir()
	path := filepath.Join(xdg, "hurl", "config")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, config.Env{"XDG_CONFIG_HOME": xdg}
}

// TestConfigFileRunSettings checks that every run setting the config file
// can set reaches the run, and that the environment and flags replace it.
func TestConfigFileRunSettings(t *testing.T) {
	path, env := writeConfig(t, `--compressed
--connect-timeout 3
--continue-on-error
--delay 20
--http2
--insecure
--ipv6
--jobs 3
--limit-rate 100
--location-trusted
--max-filesize 1000
--max-time 9s
--no-assert
--no-cookie-store
--no-proxy example.org
--proxy localhost:3128
--retry 2
--retry-interval 50
--secret token=s3cret
--test
--user bob:pw
--variable host=example.org
--very-verbose
`)
	p := mustNew(t, &Invocation{}, env)
	h, o := p.Options.HTTP, p.Options
	checks := []struct {
		name string
		ok   bool
	}{
		{"compressed", h.Compressed},
		{"connect-timeout", h.ConnectTimeout == 3*time.Second},
		{"continue-on-error", o.ContinueOnError},
		{"delay", o.Delay == 20*time.Millisecond},
		{"http2", h.HTTPVersion == engine.HTTP2},
		{"insecure", h.Insecure},
		{"ipv6", h.IPResolve == engine.IPv6},
		{"jobs", p.Workers == 3},
		{"limit-rate", h.LimitRate == 100},
		{"location-trusted", h.FollowLocation && h.LocationTrusted},
		{"max-filesize", h.MaxFilesize == 1000},
		{"max-time", h.Timeout == 9*time.Second},
		{"no-assert", o.NoAssert},
		{"no-cookie-store", o.NoCookieStore},
		{"no-proxy", h.NoProxy == "example.org"},
		{"proxy", h.Proxy == "localhost:3128"},
		{"retry", o.Retry == 2},
		{"retry-interval", o.RetryInterval == 50*time.Millisecond},
		{"secret", o.Secrets["token"] == "s3cret"},
		{"test", p.Test && p.Parallel},
		{"user", h.User == "bob:pw"},
		{"variable", fmt.Sprint(o.Variables["host"]) == "example.org"},
		{"very-verbose", o.Verbosity == engine.VeryVerbose},
	}
	for _, c := range checks {
		if !c.ok {
			t.Errorf("config file --%s not applied", c.name)
		}
	}
	// Every setting is listed with the file as its origin; a secret by
	// name only.
	if len(p.Provenance) != len(checks) {
		t.Errorf("provenance has %d settings, want %d: %v", len(p.Provenance), len(checks), p.Provenance)
	}
	for _, s := range p.Provenance {
		if s.Origin != path || strings.Contains(s.Setting, "s3cret") || strings.Contains(s.Setting, "pw") {
			t.Errorf("provenance %v", s)
		}
	}

	env["HURL_INSECURE"] = "false"
	env["HURL_RETRY"] = "5"
	p = mustNew(t, &Invocation{MaxTime: "1", Set: set("max-time")}, env)
	if p.Options.HTTP.Insecure || p.Options.Retry != 5 || p.Options.HTTP.Timeout != time.Second {
		t.Errorf("environment and flags lose to the config file: %+v", p.Options.HTTP)
	}
	for _, s := range p.Provenance {
		if s.Setting == "--max-time" || (s.Setting == "--insecure" && s.Origin == path) || (s.Setting == "--retry" && s.Origin == path) {
			t.Errorf("replaced setting listed as from the config file: %v", s)
		}
	}
}

// TestConfigFileLocationTrusted checks that HURL_LOCATION=false stops a
// config file's --location-trusted from following (and so from sending
// credentials to another host), while the flag still implies --location.
func TestConfigFileLocationTrusted(t *testing.T) {
	_, env := writeConfig(t, "--location-trusted\n")
	env["HURL_LOCATION"] = "false"
	h := mustNew(t, &Invocation{}, env).Options.HTTP
	if h.FollowLocation || h.LocationTrusted {
		t.Errorf("HURL_LOCATION=false: follow %v, trusted %v", h.FollowLocation, h.LocationTrusted)
	}
	h = mustNew(t, &Invocation{LocationTrusted: true, Set: set("location-trusted")}, env).Options.HTTP
	if !h.FollowLocation || !h.LocationTrusted {
		t.Errorf("--location-trusted: follow %v, trusted %v", h.FollowLocation, h.LocationTrusted)
	}
}

// TestConfigFileProvenanceParsedEnv checks that an environment variable
// hides a config file setting only when it replaces its value.
func TestConfigFileProvenanceParsedEnv(t *testing.T) {
	path, env := writeConfig(t, "--insecure\n--jobs 2\n--http2\n")
	env["HURL_INSECURE"] = "yes" // not a boolean: the file's value applies
	env["HURL_JOBS"] = "0"
	env["HURL_HTTP10"] = "false"
	p := mustNew(t, &Invocation{}, env)
	want := []Source{{"--insecure", path}, {"--jobs", path}, {"--http2", path}}
	var got []Source
	for _, s := range p.Provenance {
		if s.Origin == path {
			got = append(got, s)
		}
	}
	if !reflect.DeepEqual(got, want) || !p.Options.HTTP.Insecure {
		t.Errorf("provenance %v, want %v", got, want)
	}
}

// TestConfigFileLegacyPath checks the warning for a file left where
// 8.0.1 looked when XDG_CONFIG_HOME is unset.
func TestConfigFileLegacyPath(t *testing.T) {
	home := t.TempDir()
	old := filepath.Join(home, "config", "hurl", "config")
	if err := os.MkdirAll(filepath.Dir(old), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("--insecure\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := mustNew(t, &Invocation{}, config.Env{"HOME": home})
	if p.Options.HTTP.Insecure || len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], old) {
		t.Errorf("insecure %v, warnings %v", p.Options.HTTP.Insecure, p.Warnings)
	}
}

// TestConfigFileSecretsAndVariables checks the merge order of variables
// (config file < env < flags, the last wins) and that a secret name the
// config file defines cannot be given again.
func TestConfigFileSecretsAndVariables(t *testing.T) {
	_, env := writeConfig(t, "--variable a=file\n--variable b=file\n--secret c=12345678")
	env["HURL_VARIABLE_b"] = "env"
	p := mustNew(t, &Invocation{Variables: []string{"a=flag"}, Secrets: []string{"d=x"}}, env)
	if fmt.Sprint(p.Options.Variables["a"]) != "flag" || fmt.Sprint(p.Options.Variables["b"]) != "env" {
		t.Errorf("variables %v", p.Options.Variables)
	}
	if p.Options.Secrets["c"] != "12345678" || p.Options.Secrets["d"] != "x" {
		t.Errorf("secrets are not merged: %d", len(p.Options.Secrets))
	}
	_, err := New(&Invocation{Secrets: []string{"c=other"}}, env, "test")
	if err == nil || err.Error() != "secret 'c' can't be reassigned" {
		t.Errorf("secret given twice: %v", err)
	}
}

// TestConfigFileHTTP10 checks the config file's --http1.0, and that a
// flag choosing another version replaces it.
func TestConfigFileHTTP10(t *testing.T) {
	_, env := writeConfig(t, "--http1.0\n")
	if p := mustNew(t, &Invocation{}, env); p.Options.HTTP.HTTPVersion != engine.HTTP10 {
		t.Errorf("version %v", p.Options.HTTP.HTTPVersion)
	}
	if p := mustNew(t, &Invocation{HTTP2: true, Set: set("http2")}, env); p.Options.HTTP.HTTPVersion != engine.HTTP2 {
		t.Errorf("flag: version %v", p.Options.HTTP.HTTPVersion)
	}
}

// TestNewOptions checks the options added by the reference's 8.1.0 from
// each source: the config file, the environment (lists separated by
// "|") and the flags, which add to the lists and replace the rest.
func TestNewOptions(t *testing.T) {
	path, env := writeConfig(t, "--fail-with-body\n--no-header Accept\n--no-jsonpath-coercion\n--proxy-header A:b\n")
	p := mustNew(t, &Invocation{}, env)
	h := p.Options.HTTP
	if !p.Options.FailWithBody || !p.Options.NoJSONPathCoercion ||
		!reflect.DeepEqual(h.NoHeaders, []string{"Accept"}) || !reflect.DeepEqual(h.ProxyHeaders, []string{"A:b"}) {
		t.Errorf("config file: %+v", p.Options)
	}
	if want := []Source{
		{"--fail-with-body", path}, {"--no-header", path}, {"--no-jsonpath-coercion", path}, {"--proxy-header", path},
	}; !reflect.DeepEqual(p.Provenance, want) {
		t.Errorf("provenance %v", p.Provenance)
	}

	env["HURL_FAIL_WITH_BODY"] = "false"
	env["HURL_NO_HEADER"] = "User-Agent | Cookie"
	env["HURL_PROXY_HEADER"] = "C: d|E:f"
	env["HURL_HTTP2_PRIOR_KNOWLEDGE"] = "1"
	p = mustNew(t, &Invocation{NoHeader: []string{" X "}, ProxyHeader: []string{"G:h"}, NoJSONPathCoercion: false, Set: set("no-jsonpath-coercion")}, env)
	h = p.Options.HTTP
	if p.Options.FailWithBody || p.Options.NoJSONPathCoercion {
		t.Errorf("env/flag booleans: %+v", p.Options)
	}
	if want := []string{"Accept", "User-Agent", "Cookie", "X"}; !reflect.DeepEqual(h.NoHeaders, want) {
		t.Errorf("no-headers %q, want %q", h.NoHeaders, want)
	}
	if want := []string{"A:b", "C: d", "E:f", "G:h"}; !reflect.DeepEqual(h.ProxyHeaders, want) {
		t.Errorf("proxy headers %q, want %q", h.ProxyHeaders, want)
	}
	if h.HTTPVersion != engine.HTTP2PriorKnowledge {
		t.Errorf("version %v", h.HTTPVersion)
	}

	for _, tt := range []struct {
		inv  *Invocation
		env  config.Env
		want string
	}{
		{&Invocation{NoHeader: []string{"foo", ""}}, config.Env{}, "Missing header name"},
		{&Invocation{}, config.Env{"HURL_NO_HEADER": "foo|"}, "Missing header name (HURL_NO_HEADER environment variable)"},
		{&Invocation{ProxyHeader: []string{"nocolon"}}, config.Env{}, "Invalid proxy header <nocolon>, missing `:`"},
		{&Invocation{}, config.Env{"HURL_PROXY_HEADER": "a:b|c"}, "Invalid proxy header <c>, missing `:` (HURL_PROXY_HEADER environment variable)"},
	} {
		if _, err := New(tt.inv, tt.env, "test"); err == nil || err.Error() != tt.want {
			t.Errorf("%+v %v: error %v, want %q", tt.inv, tt.env, err, tt.want)
		}
	}
	p = mustNew(t, &Invocation{HTTP2Prior: true, Set: set("http2-prior-knowledge")}, config.Env{"HURL_HTTP2": "1"})
	if p.Options.HTTP.HTTPVersion != engine.HTTP2PriorKnowledge {
		t.Errorf("flag version %v", p.Options.HTTP.HTTPVersion)
	}
}

func TestVariablesAndSecrets(t *testing.T) {
	env := config.Env{"HURL_VARIABLE_host": "env-host", "SONDE_SECRET_token": "env-token", "HURL_VARIABLE_n": "1"}
	p := mustNew(t, &Invocation{Variables: []string{"n=2", "x=true"}}, env)
	if !reflect.DeepEqual(p.Options.Secrets, map[string]string{"token": "env-token"}) {
		t.Errorf("secrets %v", p.Options.Secrets)
	}
	if len(p.Options.Variables) != 3 {
		t.Errorf("variables %v", p.Options.Variables)
	}
	want := []Source{{"variable host", "HURL_VARIABLE_host"}, {"secret token", "SONDE_SECRET_token"}}
	if !reflect.DeepEqual(p.Provenance, want) {
		t.Errorf("provenance %v", p.Provenance)
	}
	if _, err := New(&Invocation{Variables: []string{"token=x"}}, env, "test"); err == nil {
		t.Error("a variable named like a secret is not refused")
	}
}

func TestErrors(t *testing.T) {
	var ue *UnsupportedError
	if _, err := New(&Invocation{SSLNoRevoke: true}, nil, "t"); !errors.As(err, &ue) || err.Error() != `option "ssl-no-revoke" is not supported by sonde yet` {
		t.Errorf("ssl-no-revoke: %v", err)
	}
	p, err := New(&Invocation{NTLM: true, Digest: true, Negotiate: true, AWSSigV4: "aws:amz:r:s"}, nil, "t")
	if h := p.Options.HTTP; err != nil || !h.NTLM || !h.Digest || !h.Negotiate || h.AWSSigV4 != "aws:amz:r:s" {
		t.Errorf("auth schemes: %v", err)
	}
	if p, err := New(&Invocation{HTTP10: true, Set: set("http1.0")}, nil, "t"); err != nil || p.Options.HTTP.HTTPVersion != engine.HTTP10 {
		t.Errorf("http1.0: %v", err)
	}
	_, err = New(&Invocation{MaxTime: "abc", Set: set("max-time")}, nil, "t")
	if err == nil || !strings.HasPrefix(err.Error(), "invalid value 'abc' for '--max-time <SECONDS>'") {
		t.Errorf("max-time: %v", err)
	}
	if _, err := New(&Invocation{Header: []string{"nocolon"}}, nil, "t"); err == nil || errors.As(err, &ue) {
		t.Errorf("header: %v", err)
	}
}

func TestTestModeAndWorkers(t *testing.T) {
	p := mustNew(t, &Invocation{Cmd: "test", Jobs: 3, Set: set("jobs")}, nil)
	if !p.Test || !p.Parallel || p.Workers != 3 {
		t.Errorf("test %v parallel %v workers %d", p.Test, p.Parallel, p.Workers)
	}
	if p := mustNew(t, &Invocation{Jobs: 3, Set: set("jobs")}, nil); p.Workers != 1 {
		t.Errorf("sequential run has %d workers", p.Workers)
	}
	if p := mustNew(t, &Invocation{}, config.Env{"SONDE_DEFAULT_USER_AGENT": "ua/1"}); p.Options.DefaultUserAgent != "ua/1" {
		t.Errorf("default UA %q", p.Options.DefaultUserAgent)
	}
	if p := mustNew(t, &Invocation{}, nil); p.Options.DefaultUserAgent != "sonde/test" {
		t.Errorf("default UA %q", p.Options.DefaultUserAgent)
	}
}

// TestResolveJobs resolves a file's sonde.yaml environment into its job,
// below the flags, and repeats the jobs.
func TestResolveJobs(t *testing.T) {
	dir := t.TempDir()
	yaml := "version: 1\ndefaults:\n  env: dev\n  jobs: 5\nenvironments:\n  dev:\n    variables:\n      host: dev.test\n    secrets_files: [dev.secrets]\n"
	if err := os.WriteFile(filepath.Join(dir, "sonde.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dev.secrets"), []byte("key=k1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "a.hurl")
	p := mustNew(t, &Invocation{Repeat: "2", Parallel: true, Set: set("repeat", "parallel")}, config.Env{})
	if err := p.Resolve(context.Background(), []Input{{Name: file}, {Stdin: true}}); err != nil {
		t.Fatal(err)
	}
	if p.Workers != 5 {
		t.Errorf("defaults.jobs: %d workers", p.Workers)
	}
	var jobs []engine.Job
	for j := range p.Jobs([]byte("GET http://x\n"), new(error)) {
		jobs = append(jobs, j)
	}
	if len(jobs) != 4 || jobs[1].Name != "-" || string(jobs[1].Source) != "GET http://x\n" {
		t.Fatalf("jobs %+v", jobs)
	}
	if jobs[0].Variables["host"] == nil || jobs[0].Secrets["key"] != "k1" {
		t.Errorf("job values %v %v", jobs[0].Variables, jobs[0].Secrets)
	}
	p = mustNew(t, &Invocation{Env: "nope", Set: set("env")}, config.Env{})
	if err := p.Resolve(context.Background(), []Input{{Name: file}}); err == nil {
		t.Error("an unknown environment is not refused")
	}
	p = mustNew(t, &Invocation{Env: "dev", Set: set("env")}, config.Env{})
	if err := p.Resolve(context.Background(), []Input{{Name: filepath.Join(t.TempDir(), "b.hurl")}}); err == nil {
		t.Error("--env without a sonde.yaml is not refused")
	}
}

// TestResolveDeclaredSecrets: on a fresh clone (no secrets file) a run
// with every "secrets:" name set elsewhere (CI: SONDE_SECRET_*) resolves;
// one without says which secret is missing and where to set it.
func TestResolveDeclaredSecrets(t *testing.T) {
	dir := t.TempDir()
	yaml := "version: 1\nenvironments:\n  local:\n    secrets_files: [secrets/local.secrets]\n    secrets: [nmk-cookie]\n"
	if err := os.WriteFile(filepath.Join(dir, "sonde.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	file := []Input{{Name: filepath.Join(dir, "a.hurl")}}
	p := mustNew(t, &Invocation{Env: "local", Set: set("env")}, config.Env{"SONDE_SECRET_nmk-cookie": "c=1"})
	if err := p.Resolve(context.Background(), file); err != nil {
		t.Fatalf("set by SONDE_SECRET_: %v", err)
	}
	p = mustNew(t, &Invocation{Env: "local", Set: set("env")}, config.Env{})
	err := p.Resolve(context.Background(), file)
	if err == nil || !strings.Contains(err.Error(), "secret nmk-cookie not set: add it to secrets/local.secrets, or set SONDE_SECRET_nmk-cookie") {
		t.Errorf("missing: %v", err)
	}
}
