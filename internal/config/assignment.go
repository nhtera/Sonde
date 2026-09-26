// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
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

// ParseAssignment parses "name=value" text (as given to --variable or
// --secret) under kind.
func ParseAssignment(s string, kind TypeKind) (Assignment, error) {
	name, raw, ok := strings.Cut(s, "=")
	if !ok {
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
