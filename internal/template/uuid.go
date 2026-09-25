// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"crypto/rand"
	"encoding/hex"
)

// newUUID returns a random (version 4, RFC 9562) UUID in lowercase
// hyphenated form.
func newUUID() string {
	var u [16]byte
	_, _ = rand.Read(u[:]) // never fails (crypto/rand panics instead)
	u[6] = u[6]&0x0f | 0x40
	u[8] = u[8]&0x3f | 0x80
	var b [36]byte
	hex.Encode(b[0:8], u[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], u[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], u[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], u[8:10])
	b[23] = '-'
	hex.Encode(b[24:], u[10:])
	return string(b[:])
}
