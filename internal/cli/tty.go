// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"io"
	"os"

	"golang.org/x/term"
)

// isTerminal reports whether w is a file attached to a terminal. Regular
// files, pipes and /dev/null are not, so colour and pretty-printing
// default off for them.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd())) //nolint:gosec // G115: a file descriptor fits in an int.
}

// binarySniffLen is how much of a body isBinary inspects.
const binarySniffLen = 2000

// isBinary reports whether body looks binary by the reference rule: a NUL
// byte in its first 2000 bytes. Text in any 8-bit encoding (Latin-1, ...)
// is not binary.
func isBinary(body []byte) bool {
	return bytes.IndexByte(body[:min(len(body), binarySniffLen)], 0) >= 0
}
