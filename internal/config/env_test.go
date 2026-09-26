// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"
	"time"

	"github.com/nhtera/sonde/internal/value"
)

func TestEnvLookup(t *testing.T) {
	e := Env{"HURL_COLOR": "1", "SONDE_COLOR": "0"}
	if v, key, ok := e.Lookup("COLOR"); !ok || v != "0" || key != "SONDE_COLOR" {
		t.Errorf("Lookup(COLOR) = %q, %q, %v, want 0, SONDE_COLOR, true (SONDE wins)", v, key, ok)
	}
	if _, _, ok := (Env{}).Lookup("COLOR"); ok {
		t.Error("Lookup(COLOR) on empty env should report not set")
	}
	e2 := Env{"HURL_COLOR": "1"}
	if v, key, ok := e2.Lookup("COLOR"); !ok || v != "1" || key != "HURL_COLOR" {
		t.Errorf("Lookup(COLOR) = %q, %q, %v, want 1, HURL_COLOR, true (HURL fallback)", v, key, ok)
	}
}

func TestEnvBool(t *testing.T) {
	tests := []struct {
		v       string
		want    bool
		wantSet bool
	}{
		{"1", true, true},
		{"true", true, true},
		{"TRUE", true, true},
		{"0", false, true},
		{"false", false, true},
		{"False", false, true},
		{"maybe", false, false},
		{"", false, false},
	}
	for _, tt := range tests {
		e := Env{"HURL_INSECURE": tt.v}
		got, ok := e.Bool("INSECURE")
		if got != tt.want || ok != tt.wantSet {
			t.Errorf("Bool(%q) = %v, %v, want %v, %v", tt.v, got, ok, tt.want, tt.wantSet)
		}
	}
	if _, ok := (Env{}).Bool("INSECURE"); ok {
		t.Error("Bool on empty env should report not set")
	}
}

func TestEnvInt(t *testing.T) {
	n, ok, err := Env{"HURL_MAX_REDIRS": "10"}.Int("MAX_REDIRS")
	if err != nil || !ok || n != 10 {
		t.Errorf("Int = %d, %v, %v, want 10, true, nil", n, ok, err)
	}
	if _, ok, err := (Env{}).Int("MAX_REDIRS"); ok || err != nil {
		t.Errorf("Int on empty env: %v, %v, want false, nil", ok, err)
	}
	if _, _, err := (Env{"HURL_MAX_REDIRS": "x"}).Int("MAX_REDIRS"); err == nil {
		t.Error("expected an error for a non-numeric value")
	}
}

func TestEnvUint(t *testing.T) {
	n, ok, err := Env{"HURL_MAX_FILESIZE": "1024"}.Uint("MAX_FILESIZE")
	if err != nil || !ok || n != 1024 {
		t.Errorf("Uint = %d, %v, %v, want 1024, true, nil", n, ok, err)
	}
	if _, _, err := (Env{"HURL_MAX_FILESIZE": "-1"}).Uint("MAX_FILESIZE"); err == nil {
		t.Error("expected an error for a negative value")
	}
}

func TestEnvString(t *testing.T) {
	if v, ok := (Env{"HURL_USER_AGENT": "sonde-test"}).String("USER_AGENT"); !ok || v != "sonde-test" {
		t.Errorf("String = %q, %v", v, ok)
	}
	if _, ok := (Env{}).String("USER_AGENT"); ok {
		t.Error("String on empty env should report not set")
	}
}

func TestEnvDuration(t *testing.T) {
	d, ok, err := Env{"HURL_DELAY": "500ms"}.Duration("DELAY", Millisecond)
	if err != nil || !ok || d != 500*time.Millisecond {
		t.Errorf("Duration = %v, %v, %v", d, ok, err)
	}
	if _, ok, err := (Env{}).Duration("DELAY", Millisecond); ok || err != nil {
		t.Errorf("Duration on empty env: %v, %v", ok, err)
	}
	if _, _, err := (Env{"HURL_DELAY": "bogus"}).Duration("DELAY", Millisecond); err == nil {
		t.Error("expected an error for an invalid duration")
	}
}

func TestEnvHeaders(t *testing.T) {
	got, ok, err := Env{"HURL_HEADER": "A:1|B:2"}.Headers()
	if err != nil || !ok || len(got) != 2 || got[0] != "A:1" || got[1] != "B:2" {
		t.Errorf("Headers = %v, %v, %v", got, ok, err)
	}
	if _, ok, err := (Env{}).Headers(); ok || err != nil {
		t.Errorf("Headers on empty env: %v, %v", ok, err)
	}
	if _, _, err := (Env{"HURL_HEADER": "no-colon"}).Headers(); err == nil {
		t.Error("expected an error for a header missing ':'")
	}
}

func TestEnvErrorFormat(t *testing.T) {
	if v, ok, err := (Env{"HURL_ERROR_FORMAT": "long"}).ErrorFormat(); err != nil || !ok || v != "long" {
		t.Errorf("ErrorFormat = %q, %v, %v", v, ok, err)
	}
	if _, _, err := (Env{"HURL_ERROR_FORMAT": "bogus"}).ErrorFormat(); err == nil {
		t.Error("expected an error for an invalid error-format")
	}
	if _, ok, err := (Env{}).ErrorFormat(); ok || err != nil {
		t.Errorf("ErrorFormat on empty env: %v, %v", ok, err)
	}
}

func TestEnvVerbosity(t *testing.T) {
	tests := []struct {
		env  Env
		want string
	}{
		{Env{"HURL_VERBOSE": "true"}, "verbose"},
		{Env{"HURL_VERY_VERBOSE": "true"}, "debug"},
		{Env{"HURL_VERBOSITY": "brief"}, "brief"},
		// verbose wins over very-verbose when both are set.
		{Env{"HURL_VERBOSE": "true", "HURL_VERY_VERBOSE": "true"}, "verbose"},
	}
	for _, tt := range tests {
		got, ok, err := tt.env.Verbosity()
		if err != nil || !ok || got != tt.want {
			t.Errorf("Verbosity(%v) = %q, %v, %v, want %q", tt.env, got, ok, err, tt.want)
		}
	}
	if _, ok, err := (Env{}).Verbosity(); ok || err != nil {
		t.Errorf("Verbosity on empty env: %v, %v", ok, err)
	}
	if _, _, err := (Env{"HURL_VERBOSITY": "loud"}).Verbosity(); err == nil {
		t.Error("expected an error for an invalid verbosity")
	}
}

func TestEnvHTTPVersion(t *testing.T) {
	tests := []struct {
		env  Env
		want string
	}{
		{Env{"HURL_HTTP3": "true"}, "3"},
		{Env{"HURL_HTTP3": "false"}, "2"},
		{Env{"HURL_HTTP2": "true"}, "2"},
		{Env{"HURL_HTTP2": "false"}, "1.1"},
		{Env{"HURL_HTTP11": "true"}, "1.1"},
		{Env{"HURL_HTTP11": "false"}, "1.0"},
		{Env{"HURL_HTTP10": "true"}, "1.0"},
	}
	for _, tt := range tests {
		got, ok := tt.env.HTTPVersion()
		if !ok || got != tt.want {
			t.Errorf("HTTPVersion(%v) = %q, %v, want %q", tt.env, got, ok, tt.want)
		}
	}
	if _, ok := (Env{"HURL_HTTP10": "false"}).HTTPVersion(); ok {
		t.Error("HURL_HTTP10=false should report unset, not http1.0")
	}
	if _, ok := (Env{}).HTTPVersion(); ok {
		t.Error("HTTPVersion on empty env should report not set")
	}
}

func TestEnvIPResolve(t *testing.T) {
	if v, ok := (Env{"HURL_IPV6": "true"}).IPResolve(); !ok || v != "6" {
		t.Errorf("IPResolve = %q, %v", v, ok)
	}
	if v, ok := (Env{"HURL_IPV6": "false"}).IPResolve(); !ok || v != "4" {
		t.Errorf("IPResolve = %q, %v", v, ok)
	}
	if v, ok := (Env{"HURL_IPV4": "true"}).IPResolve(); !ok || v != "4" {
		t.Errorf("IPResolve = %q, %v", v, ok)
	}
	if v, ok := (Env{"HURL_IPV4": "false"}).IPResolve(); !ok || v != "6" {
		t.Errorf("IPResolve = %q, %v", v, ok)
	}
	if _, ok := (Env{}).IPResolve(); ok {
		t.Error("IPResolve on empty env should report not set")
	}
}

func TestEnvFollowLocation(t *testing.T) {
	if v, err := (Env{"HURL_LOCATION": "true"}).FollowLocation(false); err != nil || !v {
		t.Errorf("FollowLocation = %v, %v", v, err)
	}
	if v, err := (Env{"HURL_LOCATION": "false"}).FollowLocation(true); err != nil || v {
		t.Errorf("FollowLocation = %v, %v", v, err)
	}
	if v, err := (Env{"HURL_LOCATION_TRUSTED": "true"}).FollowLocation(false); err != nil || !v {
		t.Errorf("FollowLocation = %v, %v", v, err)
	}
	if v, err := (Env{}).FollowLocation(true); err != nil || !v {
		t.Errorf("FollowLocation(base=true) on empty env = %v, %v", v, err)
	}
	if _, err := (Env{"HURL_LOCATION": "false", "HURL_LOCATION_TRUSTED": "true"}).FollowLocation(false); err == nil {
		t.Error("expected an error for the contradictory combination")
	}
}

func TestEnvColor(t *testing.T) {
	if got := (Env{}).Color(true); !got {
		t.Error("Color with no env vars should keep the default")
	}
	if got := (Env{"NO_COLOR": "1"}).Color(true); got {
		t.Error("NO_COLOR (any value) should disable color")
	}
	if got := (Env{"NO_COLOR": ""}).Color(true); got {
		t.Error("NO_COLOR present with an empty value should still disable color")
	}
	if got := (Env{"HURL_COLOR": "true"}).Color(false); !got {
		t.Error("HURL_COLOR=true should enable color")
	}
	if got := (Env{"HURL_COLOR": "false"}).Color(true); got {
		t.Error("HURL_COLOR=false should disable color")
	}
	if got := (Env{"HURL_NO_COLOR": "true"}).Color(true); got {
		t.Error("HURL_NO_COLOR=true should disable color")
	}
	// HURL_NO_COLOR=false is an explicit "do not disable", which enables
	// color regardless of base: !no_color wins whenever the var is set.
	if got := (Env{"HURL_NO_COLOR": "false"}).Color(false); !got {
		t.Error("HURL_NO_COLOR=false should enable color (explicit !no_color)")
	}
}

func TestEnvIsCI(t *testing.T) {
	if (Env{}).IsCI() {
		t.Error("IsCI should be false with no CI env vars")
	}
	if !(Env{"CI": ""}).IsCI() {
		t.Error("CI presence (even empty) should report IsCI")
	}
	if !(Env{"TF_BUILD": "True"}).IsCI() {
		t.Error("TF_BUILD presence should report IsCI")
	}
}

func TestEnvVariableSecretEnvVars(t *testing.T) {
	e := Env{
		"HURL_VARIABLE_foo":  "1",
		"SONDE_VARIABLE_foo": "2",
		"HURL_VARIABLE_bar":  "BAR",
		"HURL_baz":           "ignored",
		"NOT_A_VARIABLE":     "ignored",
	}
	got := e.VariableEnvVars()
	if got["foo"] != "2" {
		t.Errorf("VariableEnvVars()[foo] = %q, want 2 (SONDE wins)", got["foo"])
	}
	if got["bar"] != "BAR" {
		t.Errorf("VariableEnvVars()[bar] = %q, want BAR", got["bar"])
	}
	if len(got) != 2 {
		t.Errorf("VariableEnvVars() = %v, want 2 entries", got)
	}

	es := Env{"HURL_SECRET_a": "1", "SONDE_SECRET_a": "2"}
	gots := es.SecretEnvVars()
	if gots["a"] != "2" {
		t.Errorf("SecretEnvVars()[a] = %q, want 2 (SONDE wins)", gots["a"])
	}
}

func TestEnvApplyVariableEnvVars(t *testing.T) {
	e := Env{"HURL_VARIABLE_foo": "48"}
	vars := map[string]value.Value{"var1": value.String("zzz")}
	if err := e.ApplyVariableEnvVars(vars); err != nil {
		t.Fatal(err)
	}
	if vars["foo"] != value.Int(48) {
		t.Errorf("foo = %#v, want 48", vars["foo"])
	}
	if vars["var1"] != value.String("zzz") {
		t.Errorf("var1 was overwritten: %#v", vars["var1"])
	}
}

func TestEnvApplySecretEnvVars(t *testing.T) {
	e := Env{"HURL_SECRET_a": "A", "HURL_SECRET_b": "B"}
	secrets := map[string]string{}
	if err := e.ApplySecretEnvVars(secrets); err != nil {
		t.Fatal(err)
	}
	if secrets["a"] != "A" || secrets["b"] != "B" {
		t.Errorf("secrets = %v", secrets)
	}
}

func TestEnvUintError(t *testing.T) {
	if _, ok, err := (Env{"HURL_MAX_FILESIZE": ""}).Uint("MAX_FILESIZE"); !ok || err == nil {
		t.Errorf("Uint(\"\") = %v, %v, want ok=true and an error", ok, err)
	}
}

func TestEnvApplyVariableEnvVarsError(t *testing.T) {
	e := Env{"HURL_VARIABLE_foo": `"unterminated`}
	if err := e.ApplyVariableEnvVars(map[string]value.Value{}); err == nil {
		t.Fatal("expected an error for a malformed value")
	}
}

func TestEnvApplySecretEnvVarsDuplicate(t *testing.T) {
	e := Env{"HURL_SECRET_a": "A"}
	secrets := map[string]string{"a": "existing"}
	if err := e.ApplySecretEnvVars(secrets); err == nil {
		t.Fatal("expected a reassignment error")
	}
}

func TestFromOSEnviron(t *testing.T) {
	t.Setenv("SONDE_CONFIG_TEST_VAR", "present")
	e := FromOSEnviron()
	if v, ok := e["SONDE_CONFIG_TEST_VAR"]; !ok || v != "present" {
		t.Errorf("FromOSEnviron()[SONDE_CONFIG_TEST_VAR] = %q, %v", v, ok)
	}
}
