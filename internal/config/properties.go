// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import "strings"

// ParseProperties parses a variables/secrets file: one "name=value"
// assignment per line, blank lines and lines starting with '#' (after
// trimming) skipped. Line endings may be LF or CRLF.
func ParseProperties(data []byte, kind TypeKind) ([]Assignment, error) {
	var out []Assignment
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		a, err := ParseAssignment(line, kind)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}
