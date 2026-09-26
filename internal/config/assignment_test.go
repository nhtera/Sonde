// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/nhtera/sonde/internal/value"
)

func TestParseAssignment(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		kind     TypeKind
		wantName string
		want     value.Value
	}{
		{"inferred string", "name=Jennifer", Inferred, "name", value.String("Jennifer")},
		{"inferred bool", "female=true", Inferred, "female", value.Bool(true)},
		{"inferred int", "age=30", Inferred, "age", value.Int(30)},
		{"inferred float", "height=1.7", Inferred, "height", value.Float(1.7)},
		{"inferred quoted", `id="123"`, Inferred, "id", value.String("123")},
		{"inferred big int", "id=9223372036854775808", Inferred, "id", value.BigInt("9223372036854775808")},
		{"inferred null", "a_null=null", Inferred, "a_null", value.Null{}},
		{"forced string", "a_null=null", Forced, "a_null", value.String("null")},
		{"forced number", "id=30", Forced, "id", value.String("30")},
		{"value contains equals", "q=a=b", Inferred, "q", value.String("a=b")},
		{"empty value", "name=", Inferred, "name", value.String("")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := ParseAssignment(tt.in, tt.kind)
			if err != nil {
				t.Fatalf("ParseAssignment(%q): %v", tt.in, err)
			}
			if a.Name != tt.wantName || a.Value != tt.want {
				t.Errorf("ParseAssignment(%q) = %+v, want {%q %#v}", tt.in, a, tt.wantName, tt.want)
			}
		})
	}
}

func TestParseAssignmentMissingValue(t *testing.T) {
	_, err := ParseAssignment("name", Inferred)
	if err == nil {
		t.Fatal("expected an error for a missing '='")
	}
	const want = "Missing value for variable name!"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// TestParseAssignmentForcedMissingValueNeverEchoesValue guards a malformed
// --secret NAME=VALUE against ever printing a value: only a short,
// plain-looking name (found before the first ':' or space) may be echoed
// back to help a typo, and only when it looks like a name, not a value.
func TestParseAssignmentForcedMissingValueNeverEchoesValue(t *testing.T) {
	const secretValue = "s3cr3t-and-must-never-appear"
	_, err := ParseAssignment("token: "+secretValue, Forced)
	if err == nil {
		t.Fatal("expected an error for a secret with no '='")
	}
	if strings.Contains(err.Error(), secretValue) {
		t.Fatalf("error leaked the secret value: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "token") {
		t.Errorf("error = %q, want it to name the plausible name %q", err.Error(), "token")
	}
	if !errors.Is(err, errMissingAssignmentValue) {
		t.Error("error does not wrap errMissingAssignmentValue")
	}
}

// TestParseAssignmentForcedMissingValueNoPlausibleName checks that no
// hint is added when nothing before the first ':'/space looks like a
// name: erring toward printing nothing rather than a chunk of garbage.
func TestParseAssignmentForcedMissingValueNoPlausibleName(t *testing.T) {
	_, err := ParseAssignment("!!!not-a-name!!! rest of the line", Forced)
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "not-a-name") || strings.Contains(err.Error(), "rest of the line") {
		t.Errorf("error = %q, want no part of the line echoed back", err.Error())
	}
}

func TestParseAssignmentInferredMissingValueKeepsUpstreamWording(t *testing.T) {
	_, err := ParseAssignment("name", Inferred)
	const want = "Missing value for variable name!"
	if err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q (upstream wording, --variable is not a secret)", err, want)
	}
}

func TestParseAssignmentReserved(t *testing.T) {
	for _, kind := range []TypeKind{Inferred, Forced} {
		if _, err := ParseAssignment("newUuid=x", kind); err == nil {
			t.Errorf("expected a reserved-name error for kind %v", kind)
		}
	}
}

func TestIsReserved(t *testing.T) {
	for _, name := range []string{"getEnv", "newDate", "newUuid"} {
		if !IsReserved(name) {
			t.Errorf("IsReserved(%q) = false, want true", name)
		}
	}
	if IsReserved("foo") {
		t.Error("IsReserved(foo) = true, want false")
	}
}
