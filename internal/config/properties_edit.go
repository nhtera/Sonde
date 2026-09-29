// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/nhtera/sonde/internal/value"
)

// Properties edits change one assignment of a variables/secrets file and
// keep every other byte: comments, blank lines, order, line endings.

// newline returns the line ending data uses: CRLF when its first line
// ends with one.
func newline(data []byte) string {
	if i := bytes.IndexByte(data, '\n'); i > 0 && data[i-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}

// propertyLines returns the byte ranges of the assignment lines of name
// in data, each line with its line ending.
func propertyLines(data []byte, name string) [][2]int {
	var out [][2]int
	for start := 0; start < len(data); {
		end := len(data)
		if i := bytes.IndexByte(data[start:], '\n'); i >= 0 {
			end = start + i + 1
		}
		line := strings.TrimSpace(strings.TrimRight(string(data[start:end]), "\r\n"))
		if line != "" && !strings.HasPrefix(line, "#") {
			if n, _, ok := strings.Cut(line, "="); ok && n == name {
				out = append(out, [2]int{start, end})
			}
		}
		start = end
	}
	return out
}

// checkAssignment refuses a name or value text that would not read back
// as one assignment of name.
func checkAssignment(name, raw string) error {
	switch {
	case name == "" || strings.ContainsAny(name, "=\r\n") || strings.HasPrefix(name, "#") || strings.TrimSpace(name) != name:
		return fmt.Errorf("invalid variable name %q", name)
	case IsReserved(name):
		return fmt.Errorf("Variable %s conflicts with the %s function, use a different name.", name, name) //nolint:staticcheck,revive // the CLI's wording
	case strings.ContainsAny(raw, "\r\n"):
		return fmt.Errorf("variable %s: a multi-line value can not be written to a properties file", name)
	case strings.TrimSpace(raw) != raw:
		return fmt.Errorf("variable %s: leading or trailing spaces would be lost in a properties file", name)
	}
	return nil
}

// SetProperty returns data with the assignment of name set to raw (the
// text after '='): the line of its last assignment (the one that wins) is
// replaced, or a line is appended.
func SetProperty(data []byte, name, raw string) ([]byte, error) {
	if err := checkAssignment(name, raw); err != nil {
		return nil, err
	}
	nl := newline(data)
	line := name + "=" + raw
	lines := propertyLines(data, name)
	var out []byte
	if len(lines) == 0 {
		out = append(out, data...)
		if len(out) > 0 && out[len(out)-1] != '\n' {
			out = append(out, nl...)
		}
		out = append(out, line+nl...)
	} else {
		last := lines[len(lines)-1]
		ending := string(data[last[0]:last[1]])
		ending = ending[len(strings.TrimRight(ending, "\r\n")):]
		out = append(out, data[:last[0]]...)
		out = append(out, line+ending...)
		out = append(out, data[last[1]:]...)
	}
	return out, nil
}

// RemoveProperty returns data without any assignment of name.
func RemoveProperty(data []byte, name string) []byte {
	var out []byte
	prev := 0
	for _, l := range propertyLines(data, name) {
		out = append(out, data[prev:l[0]]...)
		prev = l[1]
	}
	return append(out, data[prev:]...)
}

// PropertyText is the text of v in a variables file that reads back as v
// (ParseProperties, Inferred): a string that would read as another type
// ("true", "42") or with its quotes stripped is double-quoted.
func PropertyText(v value.Value) (string, error) {
	switch v := v.(type) {
	case value.String:
		s := string(v)
		if got, err := InferValue(s); err == nil && value.Equal(got, v) && got.Kind() == v.Kind() {
			return s, nil
		}
		return `"` + s + `"`, nil
	case value.Bool, value.Int, value.Float, value.BigInt, value.Null:
		return value.Display(v), nil
	}
	return "", fmt.Errorf("a %s can not be written to a properties file", v.Kind())
}
