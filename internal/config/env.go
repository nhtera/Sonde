// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nhtera/sonde/internal/value"
)

// Env is a snapshot of process environment variables, or a fake one built
// for tests. Every option accessor mirrors an upstream HURL_<NAME>
// variable, with a SONDE_<NAME> variant of the same name taking precedence
// when both are set.
type Env map[string]string

// FromOSEnviron snapshots the current process environment.
func FromOSEnviron() Env {
	e := Env{}
	for _, kv := range os.Environ() {
		if name, val, ok := strings.Cut(kv, "="); ok {
			e[name] = val
		}
	}
	return e
}

// Lookup returns the value of SONDE_<name> or, if unset, HURL_<name>, and
// the actual variable name it came from (for error messages).
func (e Env) Lookup(name string) (val, key string, ok bool) {
	if v, ok := e["SONDE_"+name]; ok {
		return v, "SONDE_" + name, true
	}
	if v, ok := e["HURL_"+name]; ok {
		return v, "HURL_" + name, true
	}
	return "", "", false
}

// Bool reads a boolean option: "1"/"true" (case-insensitive) is true,
// "0"/"false" is false, anything else (including absence) reports not set.
func (e Env) Bool(name string) (v, ok bool) {
	raw, _, present := e.Lookup(name)
	if !present {
		return false, false
	}
	switch strings.ToLower(raw) {
	case "1", "true":
		return true, true
	case "0", "false":
		return false, true
	}
	return false, false
}

// Int reads an integer option.
func (e Env) Int(name string) (v int64, ok bool, err error) {
	raw, key, present := e.Lookup(name)
	if !present {
		return 0, false, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, true, fmt.Errorf("%s (%s environment variable)", err, key)
	}
	return n, true, nil
}

// Uint reads a non-negative integer option.
func (e Env) Uint(name string) (v uint64, ok bool, err error) {
	raw, key, present := e.Lookup(name)
	if !present {
		return 0, false, nil
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, true, fmt.Errorf("%s (%s environment variable)", err, key)
	}
	return n, true, nil
}

// String reads a plain string option.
func (e Env) String(name string) (v string, ok bool) {
	raw, _, present := e.Lookup(name)
	return raw, present
}

// Duration reads a duration option in defaultUnit when its text has no
// suffix.
func (e Env) Duration(name string, defaultUnit DurationUnit) (d time.Duration, ok bool, err error) {
	raw, key, present := e.Lookup(name)
	if !present {
		return 0, false, nil
	}
	d, err = ParseDuration(raw, defaultUnit)
	if err != nil {
		return 0, true, fmt.Errorf("%s (%s environment variable)", err, key)
	}
	return d, true, nil
}

// Headers reads HURL_HEADER/SONDE_HEADER: one or more "Name:value" pairs
// separated by '|'.
func (e Env) Headers() (headers []string, ok bool, err error) {
	raw, key, present := e.Lookup("HEADER")
	if !present {
		return nil, false, nil
	}
	headers = strings.Split(raw, "|")
	for _, h := range headers {
		if !strings.Contains(h, ":") {
			return nil, true, fmt.Errorf("Invalid header <%s>, missing `:` (%s environment variable)", h, key)
		}
	}
	return headers, true, nil
}

// ErrorFormat reads HURL_ERROR_FORMAT/SONDE_ERROR_FORMAT: "short" or
// "long".
func (e Env) ErrorFormat() (format string, ok bool, err error) {
	raw, key, present := e.Lookup("ERROR_FORMAT")
	if !present {
		return "", false, nil
	}
	if raw != "short" && raw != "long" {
		return "", true, fmt.Errorf("Invalid value '%s' for error-format [possible values: long, short] (%s environment variable)", raw, key)
	}
	return raw, true, nil
}

// Verbosity resolves "brief", "verbose" or "debug" from HURL_VERBOSE, then
// HURL_VERY_VERBOSE, then HURL_VERBOSITY (SONDE_ variants take precedence
// per option, as elsewhere).
func (e Env) Verbosity() (level string, ok bool, err error) {
	if v, has := e.Bool("VERBOSE"); has && v {
		return "verbose", true, nil
	}
	if v, has := e.Bool("VERY_VERBOSE"); has && v {
		return "debug", true, nil
	}
	raw, key, present := e.Lookup("VERBOSITY")
	if !present {
		return "", false, nil
	}
	switch raw {
	case "brief", "verbose", "debug":
		return raw, true, nil
	}
	return "", true, fmt.Errorf("Invalid value '%s' for verbosity [possible values: brief, verbose, debug] (%s environment variable)", raw, key)
}

// HTTPVersion resolves "1.0", "1.1", "2" or "3" from HURL_HTTP3, then
// HURL_HTTP2, then HURL_HTTP11, then HURL_HTTP10, mirroring the cascading
// rule of each flag implying the version below it when set false.
func (e Env) HTTPVersion() (version string, ok bool) {
	if v, has := e.Bool("HTTP3"); has {
		if v {
			return "3", true
		}
		return "2", true
	}
	if v, has := e.Bool("HTTP2"); has {
		if v {
			return "2", true
		}
		return "1.1", true
	}
	if v, has := e.Bool("HTTP11"); has {
		if v {
			return "1.1", true
		}
		return "1.0", true
	}
	if v, has := e.Bool("HTTP10"); has && v {
		return "1.0", true
	}
	return "", false
}

// IPResolve resolves "4" or "6" from HURL_IPV6, then HURL_IPV4.
func (e Env) IPResolve() (family string, ok bool) {
	if v, has := e.Bool("IPV6"); has {
		if v {
			return "6", true
		}
		return "4", true
	}
	if v, has := e.Bool("IPV4"); has {
		if v {
			return "4", true
		}
		return "6", true
	}
	return "", false
}

// FollowLocation resolves --location from HURL_LOCATION/HURL_LOCATION_TRUSTED,
// erroring on the contradictory combination location=false,
// location-trusted=true.
func (e Env) FollowLocation(base bool) (bool, error) {
	loc, locSet := e.Bool("LOCATION")
	trusted, trustedSet := e.Bool("LOCATION_TRUSTED")
	switch {
	case locSet && loc:
		return true, nil
	case locSet && !loc && trustedSet && trusted:
		return false, fmt.Errorf("Invalid environment variables configuration HURL_LOCATION HURL_LOCATION_TRUSTED")
	case locSet && !loc:
		return false, nil
	case !locSet && trustedSet && trusted:
		return true, nil
	}
	return base, nil
}

// Color resolves color on/off: NO_COLOR (any value, standard convention)
// disables; else HURL_NO_COLOR/SONDE_NO_COLOR; else HURL_COLOR/SONDE_COLOR;
// else base.
func (e Env) Color(base bool) bool {
	if _, present := e["NO_COLOR"]; present {
		return false
	}
	if v, ok := e.Bool("NO_COLOR"); ok {
		return !v
	}
	if v, ok := e.Bool("COLOR"); ok {
		return v
	}
	return base
}

// IsCI reports whether CI or TF_BUILD is set, signalling a CI environment.
func (e Env) IsCI() bool {
	_, ci := e["CI"]
	_, azure := e["TF_BUILD"]
	return ci || azure
}

// VariableEnvVars returns every HURL_VARIABLE_<name>/SONDE_VARIABLE_<name>
// as name/raw-value pairs; a SONDE_VARIABLE_<name> wins over
// HURL_VARIABLE_<name> of the same name.
func (e Env) VariableEnvVars() map[string]string {
	return e.prefixedEnvVars("HURL_VARIABLE_", "SONDE_VARIABLE_")
}

// SecretEnvVars returns every HURL_SECRET_<name>/SONDE_SECRET_<name> as
// name/raw-value pairs; a SONDE_SECRET_<name> wins over
// HURL_SECRET_<name> of the same name.
func (e Env) SecretEnvVars() map[string]string {
	return e.prefixedEnvVars("HURL_SECRET_", "SONDE_SECRET_")
}

func (e Env) prefixedEnvVars(hurlPrefix, sondePrefix string) map[string]string {
	out := map[string]string{}
	for name, v := range e {
		if rest, ok := strings.CutPrefix(name, hurlPrefix); ok && rest != "" {
			out[rest] = v
		}
	}
	for name, v := range e {
		if rest, ok := strings.CutPrefix(name, sondePrefix); ok && rest != "" {
			out[rest] = v
		}
	}
	return out
}

// ApplyVariableEnvVars inserts every HURL_VARIABLE_*/SONDE_VARIABLE_* into
// variables, inferring each value's type.
func (e Env) ApplyVariableEnvVars(variables map[string]value.Value) error {
	for name, raw := range e.VariableEnvVars() {
		v, err := InferValue(raw)
		if err != nil {
			return err
		}
		variables[name] = v
	}
	return nil
}

// ApplySecretEnvVars inserts every HURL_SECRET_*/SONDE_SECRET_* into
// secrets, erroring on a name defined twice.
func (e Env) ApplySecretEnvVars(secrets map[string]string) error {
	for name, raw := range e.SecretEnvVars() {
		if err := AddSecret(secrets, name, value.String(raw)); err != nil {
			return err
		}
	}
	return nil
}
