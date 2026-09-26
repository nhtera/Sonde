// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// netrcEntry is one `machine` (or `default`) stanza of a netrc file.
type netrcEntry struct {
	machine  string // empty for the `default` entry
	login    string
	password string
}

// netrcFile is a parsed netrc file.
type netrcFile struct {
	entries []netrcEntry
}

// lookup returns the login and password for host, preferring an exact
// `machine` match over a `default` entry.
func (f *netrcFile) lookup(host string) (login, password string, ok bool) {
	if f == nil {
		return "", "", false
	}
	var def *netrcEntry
	for i := range f.entries {
		e := &f.entries[i]
		if e.machine == "" {
			def = e
			continue
		}
		if strings.EqualFold(e.machine, host) {
			return e.login, e.password, true
		}
	}
	if def != nil {
		return def.login, def.password, true
	}
	return "", "", false
}

// defaultNetrcPath returns ~/.netrc (~/_netrc on windows), or the NETRC
// environment variable when set.
func defaultNetrcPath() string {
	if v := os.Getenv("NETRC"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	name := ".netrc"
	if os.PathSeparator == '\\' {
		name = "_netrc"
	}
	return filepath.Join(home, name)
}

// loadNetrc reads and parses a netrc file. A missing file is reported
// through ok=false rather than an error, since netrc without netrcFile is
// optional by nature (NetrcOptional decides whether that is fatal).
func loadNetrc(path string, read func(string) ([]byte, error)) (*netrcFile, error) {
	data, err := read(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return parseNetrc(string(data)), nil
}

// parseNetrc parses the tiny whitespace-tokenized netrc grammar: a
// sequence of `machine NAME`/`default` stanzas, each followed by
// `login`/`password`/`account`/`macdef` tokens up to the next stanza.
func parseNetrc(data string) *netrcFile {
	fields := strings.Fields(data)
	f := &netrcFile{}
	var cur *netrcEntry
	for i := 0; i < len(fields); i++ {
		switch fields[i] {
		case "machine":
			f.entries = append(f.entries, netrcEntry{})
			cur = &f.entries[len(f.entries)-1]
			if i+1 < len(fields) {
				i++
				cur.machine = fields[i]
			}
		case "default":
			f.entries = append(f.entries, netrcEntry{})
			cur = &f.entries[len(f.entries)-1]
		case "login":
			if cur != nil && i+1 < len(fields) {
				i++
				cur.login = fields[i]
			}
		case "password":
			if cur != nil && i+1 < len(fields) {
				i++
				cur.password = fields[i]
			}
		case "account":
			if i+1 < len(fields) {
				i++
			}
		case "macdef":
			// Skip the macro name and its body, up to a blank line; since
			// fields collapses whitespace, approximate by skipping to the
			// next known keyword.
			if i+1 < len(fields) {
				i++
			}
		}
	}
	return f
}
