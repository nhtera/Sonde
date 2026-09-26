// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/nhtera/sonde/internal/config"
)

// changed reports whether flag was explicitly given on the command line.
func changed(cmd *cobra.Command, flag string) bool {
	return cmd.Flags().Changed(flag)
}

// resolveBool applies the CLI-flag-then-env-then-default precedence of a
// "presence" boolean flag: one with no negating counterpart, true only
// when the flag itself was given.
func resolveBool(cmd *cobra.Command, flag, envName string, cliVal bool, env config.Env) bool {
	if changed(cmd, flag) {
		return cliVal
	}
	if v, ok := env.Bool(envName); ok {
		return v
	}
	return false
}

// resolveString applies the same precedence for a plain string flag.
func resolveString(cmd *cobra.Command, flag, envName string, cliVal string, env config.Env, def string) string {
	if changed(cmd, flag) {
		return cliVal
	}
	if v, ok := env.String(envName); ok {
		return v
	}
	return def
}

// resolveDuration parses a duration written as "5", "5s" or "500ms" from
// the flag if given, else the env var if set, else returns def unparsed.
func resolveDuration(cmd *cobra.Command, flag, envName, cliVal string, env config.Env, unit config.DurationUnit, def time.Duration) (time.Duration, error) {
	if changed(cmd, flag) {
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
			return 0, NewExitError(ExitUsage, err)
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
func resolveCount(cmd *cobra.Command, flag, envName, cliVal string, env config.Env, def int, placeholder string) (int, error) {
	if changed(cmd, flag) {
		n, err := parseCount(cliVal)
		if err != nil {
			return 0, invalidFlagErr(flag, placeholder, cliVal, err)
		}
		return n, nil
	}
	if raw, key, ok := env.Lookup(envName); ok {
		n, err := parseCount(raw)
		if err != nil {
			return 0, NewExitError(ExitUsage, fmt.Errorf("%v (%s environment variable)", err, key))
		}
		return n, nil
	}
	return def, nil
}

// resolveInt64 parses a byte-count-like flag (max-filesize, limit-rate).
func resolveInt64(cmd *cobra.Command, flag, envName, cliVal string, env config.Env, def int64, placeholder string) (int64, error) {
	if changed(cmd, flag) {
		n, err := strconv.ParseInt(cliVal, 10, 64)
		if err != nil {
			return 0, invalidFlagErr(flag, placeholder, cliVal, err)
		}
		return n, nil
	}
	if n, ok, err := env.Uint(envName); ok {
		if err != nil {
			return 0, NewExitError(ExitUsage, err)
		}
		return int64(n), nil //nolint:gosec // G115: a 64-bit byte count never overflows int64 in practice
	}
	return def, nil
}
