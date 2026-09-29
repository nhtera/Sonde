// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package runflags

import (
	"fmt"
	"net/url"
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
// is set, credentials are references to environment variables, and a
// first comment line names the variables to set: the value of every
// --secret (SECRET_<name>), the password of --user, --cert and a --proxy
// URL, and the value of an Authorization, Proxy-Authorization or Cookie
// --header. A value cmd.exe can not quote (a line break, `%` or `!`) is an
// error for that dialect.
func Shell(inv *runplan.Invocation, dialect Dialect, reveal bool) (string, error) {
	args := Args(inv)
	words := []string{"sonde"}
	var refs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, value, joined := strings.Cut(arg, "=")
		if !strings.HasPrefix(arg, "--") || arg == "--" {
			name, joined = "", false
		}
		_, credential := mask(name, "")
		switch {
		case reveal || !credential || !joined && i+1 == len(args):
			w, err := quote(dialect, arg)
			if err != nil {
				return "", err
			}
			words = append(words, w)
			continue
		case !joined:
			i++
			value = args[i]
			words = append(words, arg)
		}
		m, ok := mask(name, value)
		if !ok {
			w, err := quote(dialect, value)
			if err != nil {
				return "", err
			}
			if joined {
				w = name + "=" + w
			}
			words = append(words, w)
			continue
		}
		refs = append(refs, m.ref)
		w, err := reference(dialect, m)
		if err != nil {
			return "", err
		}
		if joined {
			w = name + "=" + w
		}
		words = append(words, w)
	}
	cmd := strings.Join(words, " ")
	if len(refs) == 0 {
		return cmd, nil
	}
	slices.Sort(refs)
	refs = slices.Compact(refs)
	lead := "#"
	if dialect == Cmd {
		lead = "REM"
	}
	return lead + " Set " + strings.Join(refs, ", ") + " first: secret values are not shown.\n" + cmd, nil
}

// masked is a flag value with its credential replaced by the environment
// variable ref: prefix and suffix are kept in clear.
type masked struct {
	prefix, ref, suffix string
}

// credentialHeaders are the --header names whose value is masked.
var credentialHeaders = []string{"authorization", "proxy-authorization", "cookie"}

var envSafe = regexp.MustCompile(`[^A-Za-z0-9_]`)

// mask splits value, the value of flag, around its credential. With an
// empty value it reports whether flag may hold one.
func mask(flag, value string) (masked, bool) {
	switch flag {
	case "--secret":
		if value == "" {
			return masked{}, true
		}
		name, _, ok := strings.Cut(value, "=")
		if !ok {
			return masked{ref: "SECRET_VALUE"}, true
		}
		return masked{prefix: name + "=", ref: "SECRET_" + envSafe.ReplaceAllString(name, "_")}, true
	case "--user":
		if value == "" {
			return masked{}, true
		}
		user, _, ok := strings.Cut(value, ":")
		if !ok {
			return masked{ref: "SECRET_USER"}, true
		}
		return masked{prefix: user + ":", ref: "SECRET_USER_PASSWORD"}, true
	case "--cert":
		if value == "" {
			return masked{}, true
		}
		from := 0
		if len(value) > 2 && value[1] == ':' { // a Windows drive letter
			from = 2
		}
		i := strings.LastIndexByte(value[from:], ':')
		if i < 0 {
			return masked{}, false
		}
		return masked{prefix: value[:from+i+1], ref: "SECRET_CERT_PASSWORD"}, true
	case "--proxy":
		if value == "" {
			return masked{}, true
		}
		u, err := url.Parse(value)
		if err != nil || u.User == nil {
			return masked{}, false
		}
		if _, ok := u.User.Password(); !ok {
			return masked{}, false
		}
		at := strings.Index(value, "@")
		colon := strings.LastIndex(value[:at], ":")
		return masked{prefix: value[:colon+1], ref: "SECRET_PROXY_PASSWORD", suffix: value[at:]}, true
	case "--header":
		if value == "" {
			return masked{}, true
		}
		name, rest, ok := strings.Cut(value, ":")
		if !ok || !slices.Contains(credentialHeaders, strings.ToLower(strings.TrimSpace(name))) {
			return masked{}, false
		}
		sp := rest[:len(rest)-len(strings.TrimLeft(rest, " "))]
		ref := "SECRET_HEADER_" + strings.ToUpper(envSafe.ReplaceAllString(strings.TrimSpace(name), "_"))
		return masked{prefix: name + ":" + sp, ref: ref}, true
	}
	return masked{}, false
}

// reference is one shell word: m's prefix and suffix in clear around the
// value of the environment variable m.ref.
func reference(dialect Dialect, m masked) (string, error) {
	switch dialect {
	case PowerShell:
		esc := strings.NewReplacer("`", "``", `"`, "`\"", "$", "`$", "“", "`“", "”", "`”", "„", "`„")
		return `"` + esc.Replace(m.prefix) + "${env:" + m.ref + "}" + esc.Replace(m.suffix) + `"`, nil
	case Cmd:
		if err := cmdSafe(m.prefix + m.suffix); err != nil {
			return "", err
		}
		return `"` + strings.ReplaceAll(m.prefix, `"`, `""`) + "%" + m.ref + "%" + strings.ReplaceAll(m.suffix, `"`, `""`) + `"`, nil
	}
	w := `"${` + m.ref + `}"`
	if m.prefix != "" {
		p, _ := quote(POSIX, m.prefix)
		w = p + w
	}
	if m.suffix != "" {
		s, _ := quote(POSIX, m.suffix)
		w += s
	}
	return w, nil
}

// Characters a word may hold unquoted, per dialect.
var (
	posixPlain = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)
	psPlain    = regexp.MustCompile(`^[A-Za-z0-9_+=:./-]+$`)
	cmdPlain   = regexp.MustCompile(`^[A-Za-z0-9_+=:,./\\-]+$`)
)

// cmdSafe refuses a value cmd.exe can not hold in double quotes: a line
// break ends the command, `%` and `!` expand variables.
func cmdSafe(s string) error {
	if strings.ContainsAny(s, "\r\n%!") {
		return fmt.Errorf("runflags: %q can not be written for cmd.exe (a line break, %% or !): use PowerShell", s)
	}
	return nil
}

// quote renders s as one shell word.
func quote(dialect Dialect, s string) (string, error) {
	switch dialect {
	case PowerShell:
		if psPlain.MatchString(s) {
			return s, nil
		}
		// PowerShell ends a single-quoted string at any single quote,
		// typographic ones included: each is doubled.
		esc := strings.NewReplacer("'", "''", "‘", "‘‘", "’", "’’", "‚", "‚‚", "‛", "‛‛")
		return "'" + esc.Replace(s) + "'", nil
	case Cmd:
		if cmdPlain.MatchString(s) {
			return s, nil
		}
		if err := cmdSafe(s); err != nil {
			return "", err
		}
		// Inside double quotes cmd.exe leaves & | < > ^ alone; a quote is
		// doubled, which keeps cmd.exe inside the string and gives the
		// program one quote.
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`, nil
	}
	if posixPlain.MatchString(s) {
		return s, nil
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'", nil
}
