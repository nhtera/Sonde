// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runflags

import (
	"regexp"
	"slices"
	"strings"

	"github.com/nhtera/sonde/internal/runplan"
)

// Dialect is a shell a command is written for.
type Dialect int

// Dialects.
const (
	POSIX      Dialect = iota // sh, bash, zsh
	PowerShell                // Windows PowerShell and pwsh
	Cmd                       // Windows cmd.exe
)

// Shell renders inv as a `sonde` command line for dialect. Unless reveal
// is set, the value of every --secret and the password of --user are
// references to environment variables (SECRET_<name>,
// SECRET_USER_PASSWORD), and a first comment line names the variables to
// set. In cmd.exe a `%` in a value can not be escaped inside quotes: such
// a command is best run from a script, where `%%` would be needed.
func Shell(inv *runplan.Invocation, dialect Dialect, reveal bool) string {
	args := Args(inv)
	words := make([]string, 0, len(args)+1)
	words = append(words, "sonde")
	var refs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, value, joined := strings.Cut(arg, "=")
		secretFlag := name == "--secret" || name == "--user"
		switch {
		case reveal || !secretFlag || !joined && i+1 == len(args):
			words = append(words, quote(dialect, arg))
			continue
		case !joined:
			i++
			value = args[i]
			words = append(words, arg)
		}
		key, sep, ref := maskedParts(name, value)
		refs = append(refs, ref)
		word := reference(dialect, key+sep, ref)
		if joined {
			word = quote(dialect, name+"=") + word
		}
		words = append(words, word)
	}
	cmd := strings.Join(words, " ")
	if len(refs) == 0 {
		return cmd
	}
	slices.Sort(refs)
	refs = slices.Compact(refs)
	lead := "#"
	if dialect == Cmd {
		lead = "REM"
	}
	return lead + " Set " + strings.Join(refs, ", ") + " first: secret values are not shown.\n" + cmd
}

// maskedParts splits the value of a --secret (name=value) or --user
// (user:password) flag into the part kept in clear and the environment
// variable the rest is read from.
func maskedParts(flag, value string) (key, sep, ref string) {
	if flag == "--user" {
		user, _, ok := strings.Cut(value, ":")
		if !ok {
			return "", "", "SECRET_USER"
		}
		return user, ":", "SECRET_USER_PASSWORD"
	}
	name, _, ok := strings.Cut(value, "=")
	if !ok {
		return "", "", "SECRET_VALUE"
	}
	return name, "=", "SECRET_" + envSafe.ReplaceAllString(name, "_")
}

var envSafe = regexp.MustCompile(`[^A-Za-z0-9_]`)

// reference is one shell word: prefix in clear, then the value of the
// environment variable ref.
func reference(dialect Dialect, prefix, ref string) string {
	switch dialect {
	case PowerShell:
		return `"` + strings.NewReplacer("`", "``", `"`, "`\"", "$", "`$").Replace(prefix) + "$env:" + ref + `"`
	case Cmd:
		return `"` + strings.ReplaceAll(prefix, `"`, `\"`) + "%" + ref + `%"`
	}
	if prefix == "" {
		return `"$` + ref + `"`
	}
	return quote(POSIX, prefix) + `"$` + ref + `"`
}

// Characters a word may hold unquoted, per dialect.
var (
	posixSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)
	psSafe    = regexp.MustCompile(`^[A-Za-z0-9_+=:./-]+$`)
	cmdSafe   = regexp.MustCompile(`^[A-Za-z0-9_+=:,./\\-]+$`)
)

// quote renders s as one shell word.
func quote(dialect Dialect, s string) string {
	switch dialect {
	case PowerShell:
		if psSafe.MatchString(s) {
			return s
		}
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	case Cmd:
		if cmdSafe.MatchString(s) {
			return s
		}
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	if posixSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
