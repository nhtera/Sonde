// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runplan

import (
	"fmt"
	"strconv"
	"time"

	"github.com/nhtera/sonde/internal/config"
)

// ResolveBool applies the flag-then-env-then-default precedence of a
// "presence" boolean flag: one with no negating counterpart, true only
// when the flag itself was given.
func ResolveBool(inv *Invocation, flag, envName string, cliVal bool, env config.Env) bool {
	return ResolveBoolOr(inv, flag, envName, cliVal, env, false)
}

// ResolveBoolOr is ResolveBool with a default (the config file's value).
func ResolveBoolOr(inv *Invocation, flag, envName string, cliVal bool, env config.Env, def bool) bool {
	if inv.Changed(flag) {
		return cliVal
	}
	if v, ok := env.Bool(envName); ok {
		return v
	}
	return def
}

// fileString is a string flag no environment variable sets: the flag if
// given, else the config file's value if it has one.
func fileString(inv *Invocation, flag, cliVal string, fileVal *string) string {
	if !inv.Changed(flag) && fileVal != nil {
		return *fileVal
	}
	return cliVal
}

// deref is *p, or the zero value when p is nil (a config file option the
// file does not set).
func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// resolveString applies the same precedence for a plain string flag.
func resolveString(inv *Invocation, flag, envName string, cliVal string, env config.Env, def string) string {
	if inv.Changed(flag) {
		return cliVal
	}
	if v, ok := env.String(envName); ok {
		return v
	}
	return def
}

// resolveDuration parses a duration written as "5", "5s" or "500ms" from
// the flag if given, else the env var if set, else returns def unparsed.
func resolveDuration(inv *Invocation, flag, envName, cliVal string, env config.Env, unit config.DurationUnit, def time.Duration) (time.Duration, error) {
	if inv.Changed(flag) {
		if cliVal == "" {
			return def, nil
		}
		d, err := config.ParseDuration(cliVal, unit)
		if err != nil {
			return 0, invalidFlagErr(flag, durationPlaceholder(unit), cliVal, err)
		}
		return d, nil
	}
	if d, ok, err := env.Duration(envName, unit); ok {
		if err != nil {
			return 0, err
		}
		return d, nil
	}
	return def, nil
}

func durationPlaceholder(unit config.DurationUnit) string {
	if unit == config.Second {
		return "SECONDS"
	}
	return "MILLISECONDS"
}

// maxCount is the largest value a "Count" flag (repeat/retry/max-redirs)
// accepts: the upstream CLI parses it as a signed 32-bit integer.
const maxCount = 1<<31 - 1

// parseCount parses a "Count" flag value: an integer no less than -1
// (repeat/retry/max-redirs all share this range; -1 means unlimited).
func parseCount(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	if n < -1 || n > maxCount {
		return 0, fmt.Errorf("%d is not in -1..=%d", n, maxCount)
	}
	return n, nil
}

// resolveCount parses a "count" flag: an integer, -1 meaning unlimited
// where the flag documents it. placeholder matches invalidFlagErr.
func resolveCount(inv *Invocation, flag, envName, cliVal string, env config.Env, def int, placeholder string) (int, error) {
	if inv.Changed(flag) {
		n, err := parseCount(cliVal)
		if err != nil {
			return 0, invalidFlagErr(flag, placeholder, cliVal, err)
		}
		return n, nil
	}
	if raw, key, ok := env.Lookup(envName); ok {
		n, err := parseCount(raw)
		if err != nil {
			return 0, fmt.Errorf("%v (%s environment variable)", err, key)
		}
		return n, nil
	}
	return def, nil
}

// resolveInt64 parses a byte-count-like flag (max-filesize, limit-rate).
func resolveInt64(inv *Invocation, flag, envName, cliVal string, env config.Env, def int64, placeholder string) (int64, error) {
	if inv.Changed(flag) {
		n, err := strconv.ParseInt(cliVal, 10, 64)
		if err != nil {
			return 0, invalidFlagErr(flag, placeholder, cliVal, err)
		}
		return n, nil
	}
	if n, ok, err := env.Uint(envName); ok {
		if err != nil {
			return 0, err
		}
		return int64(n), nil //nolint:gosec // G115: a 64-bit byte count never overflows int64 in practice
	}
	return def, nil
}

// invalidFlagErr reports a flag value that does not parse, in the
// upstream CLI's wording.
func invalidFlagErr(flag, placeholder, value string, cause error) error {
	return fmt.Errorf("invalid value '%s' for '--%s <%s>' (%v)", value, flag, placeholder, cause)
}
