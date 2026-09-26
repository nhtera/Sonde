// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nhtera/sonde/internal/value"
)

// Assignment is one "name=value" pair from the command line or a
// properties file.
type Assignment struct {
	Name  string
	Value value.Value
}

// reserved are variable names that would conflict with a template function
// of the same name.
var reserved = map[string]bool{"getEnv": true, "newDate": true, "newUuid": true}

// IsReserved reports whether name conflicts with a template function.
func IsReserved(name string) bool { return reserved[name] }

// errMissingAssignmentValue is wrapped by ParseAssignment when s has no
// '='. A secrets source (kind == Forced) never echoes s itself back in
// the resulting error message: the text after any '=' it might still be
// missing could itself be an unregistered secret value; ParseProperties
// uses errors.Is to replace this error entirely with a file/line-only one.
var errMissingAssignmentValue = errors.New("missing '='")

// ParseAssignment parses "name=value" text (as given to --variable or
// --secret) under kind.
func ParseAssignment(s string, kind TypeKind) (Assignment, error) {
	name, raw, ok := strings.Cut(s, "=")
	if !ok {
		if kind == Forced {
			return Assignment{}, fmt.Errorf("invalid secret assignment%s: %w", secretAssignmentHint(s), errMissingAssignmentValue)
		}
		return Assignment{}, fmt.Errorf("Missing value for variable %s!", s)
	}
	if IsReserved(name) {
		return Assignment{}, fmt.Errorf("Variable %s conflicts with the %s function, use a different name.", name, name)
	}
	v, err := ParseValue(raw, kind)
	if err != nil {
		return Assignment{}, err
	}
	return Assignment{Name: name, Value: v}, nil
}

// secretAssignmentHint returns " for \"name\"" when s starts with a short,
// plain-looking name up to its first ':' or space, and "" otherwise: just
// enough to help a mistyped --secret NAME=VALUE without ever risking a
// value in the message.
func secretAssignmentHint(s string) string {
	name := s
	if i := strings.IndexAny(s, ": "); i >= 0 {
		name = s[:i]
	}
	if !looksLikeAssignmentName(name) {
		return ""
	}
	return fmt.Sprintf(" for %q", name)
}

// looksLikeAssignmentName reports whether name is short and plain enough
// (letters, digits, '_', '-') to echo back safely.
func looksLikeAssignmentName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}
