// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/nhtera/sonde/internal/value"
)

func TestInferValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want value.Value
	}{
		{"string", "Jennifer", value.String("Jennifer")},
		{"true", "true", value.Bool(true)},
		{"false", "false", value.Bool(false)},
		{"null", "null", value.Null{}},
		{"int", "30", value.Int(30)},
		{"negative int", "-5", value.Int(-5)},
		{"zero", "0", value.Int(0)},
		{"big int", "9223372036854775808", value.BigInt("9223372036854775808")},
		{"float", "1.7", value.Float(1.7)},
		{"whole float", "1.0", value.Float(1.0)},
		{"negative float", "-1.0", value.Float(-1.0)},
		{"quoted", `"123"`, value.String("123")},
		{"quoted empty", `""`, value.String("")},
		{"quoted looks like bool", `"true"`, value.String("true")},
		// A negative all-digit string is not "all digits" (the sign isn't a
		// digit), so it falls through to the float branch, losing exactness:
		// this mirrors the upstream `s.chars().all(char::is_numeric)`.
		{"negative big int becomes float", "-99999999999999999999", value.Float(-1e+20)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InferValue(tt.in)
			if err != nil {
				t.Fatalf("InferValue(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("InferValue(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestInferValueUnterminatedQuote(t *testing.T) {
	if _, err := InferValue(`"123`); err == nil {
		t.Fatal("expected an error for an unterminated quote")
	}
}

func TestParseValueForced(t *testing.T) {
	tests := []string{"30", "true", "null", "1.7", `"x"`}
	for _, in := range tests {
		got, err := ParseValue(in, Forced)
		if err != nil {
			t.Fatalf("ParseValue(%q, Forced): %v", in, err)
		}
		if got != value.String(in) {
			t.Errorf("ParseValue(%q, Forced) = %#v, want the literal string", in, got)
		}
	}
}

func TestIsAllDigits(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"0", true},
		{"12345", true},
		{"-1", false},
		{"1.0", false},
		{"1a", false},
	}
	for _, tt := range tests {
		if got := isAllDigits(tt.in); got != tt.want {
			t.Errorf("isAllDigits(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
