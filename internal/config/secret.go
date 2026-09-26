// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"

	"github.com/nhtera/sonde/internal/value"
)

// AddSecret inserts name/v into secrets. It errors if name is already
// present: a secret can be defined once, from a single source.
func AddSecret(secrets map[string]string, name string, v value.Value) error {
	if _, ok := secrets[name]; ok {
		return fmt.Errorf("secret '%s' can't be reassigned", name)
	}
	s, ok := v.(value.String)
	if !ok {
		return fmt.Errorf("secret '%s' must be a string", name)
	}
	secrets[name] = string(s)
	return nil
}

// CheckNoClash errors if any name is defined as both a variable and a
// secret.
func CheckNoClash(variables map[string]value.Value, secrets map[string]string) error {
	for name := range secrets {
		if _, ok := variables[name]; ok {
			return fmt.Errorf("%s is defined as both a variable and a secret", name)
		}
	}
	return nil
}
