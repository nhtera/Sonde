// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runplan

import (
	"context"
	"errors"
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
	dir := filepath.Join(home, "config", "hurl")
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
	want := []Source{{"--header", "HURL_HEADER"}, {"--user-agent", path}, {"--max-redirs", path}, {"--verbose", path}, {"--header", path}}
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
	if _, err := New(&Invocation{NTLM: true}, nil, "t"); !errors.As(err, &ue) || err.Error() != `option "ntlm" is not supported by sonde yet` {
		t.Errorf("ntlm: %v", err)
	}
	if _, err := New(&Invocation{HTTP10: true, Set: set("http1.0")}, nil, "t"); !errors.As(err, &ue) {
		t.Errorf("http1.0: %v", err)
	}
	_, err := New(&Invocation{MaxTime: "abc", Set: set("max-time")}, nil, "t")
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
