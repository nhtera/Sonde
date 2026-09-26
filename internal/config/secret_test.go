// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/nhtera/sonde/internal/value"
)

func TestAddSecret(t *testing.T) {
	secrets := map[string]string{}
	if err := AddSecret(secrets, "a", value.String("x")); err != nil {
		t.Fatal(err)
	}
	if secrets["a"] != "x" {
		t.Errorf("secrets[a] = %q, want x", secrets["a"])
	}
}

func TestAddSecretReassign(t *testing.T) {
	secrets := map[string]string{"a": "x"}
	err := AddSecret(secrets, "a", value.String("y"))
	if err == nil {
		t.Fatal("expected an error reassigning a secret")
	}
	const want = "secret 'a' can't be reassigned"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if secrets["a"] != "x" {
		t.Error("the original value must not be overwritten")
	}
}

func TestAddSecretNonString(t *testing.T) {
	if err := AddSecret(map[string]string{}, "a", value.Int(1)); err == nil {
		t.Fatal("expected an error for a non-string value")
	}
}

func TestCheckNoClash(t *testing.T) {
	vars := map[string]value.Value{"a": value.Int(1)}
	secrets := map[string]string{"a": "x"}
	if err := CheckNoClash(vars, secrets); err == nil {
		t.Fatal("expected a clash error")
	}
	if err := CheckNoClash(vars, map[string]string{"b": "x"}); err != nil {
		t.Errorf("unexpected error for non-overlapping names: %v", err)
	}
	if err := CheckNoClash(nil, nil); err != nil {
		t.Errorf("unexpected error for empty inputs: %v", err)
	}
}
