// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"strconv"
	"strings"

	"github.com/nhtera/sonde/internal/value"
)

// TypeKind selects how a raw "name=value" text turns into a typed value.
type TypeKind int

// Type kinds.
const (
	// Inferred infers the value's type from its text (used for variables).
	Inferred TypeKind = iota
	// Forced always produces a string (used for secrets).
	Forced
)

// ParseValue turns raw text into a typed value under kind.
func ParseValue(raw string, kind TypeKind) (value.Value, error) {
	if kind == Forced {
		return value.String(raw), nil
	}
	return InferValue(raw)
}

// InferValue infers a typed value from raw text: the literals true, false
// and null, an integer, an unsigned all-digit big integer, a floating point
// number, a double-quoted string, or else the text itself as a string.
func InferValue(raw string) (value.Value, error) {
	switch raw {
	case "true":
		return value.Bool(true), nil
	case "false":
		return value.Bool(false), nil
	case "null":
		return value.Null{}, nil
	}
	if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return value.Int(i), nil
	}
	if isAllDigits(raw) {
		return value.BigInt(raw), nil
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return value.Float(f), nil
	}
	if rest, ok := strings.CutPrefix(raw, `"`); ok {
		body, ok := strings.CutSuffix(rest, `"`)
		if !ok {
			return nil, errors.New("Value should end with a double quote")
		}
		return value.String(body), nil
	}
	return value.String(raw), nil
}

// isAllDigits reports whether s is one or more ASCII digits (no sign): the
// case a signed 64-bit parse already rejected but that is still a number,
// too large to fit an int64.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
