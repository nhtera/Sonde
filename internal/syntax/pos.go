// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package syntax parses .hurl/.sonde files into a lossless AST, prints it
// back byte for byte, formats it canonically and renders diagnostics.
//
// The parser is recursive descent with backtracking: a recoverable error lets
// the caller try another alternative, a non-recoverable one stops parsing.
package syntax

import "fmt"

// Pos is a position in a source file. Offset (bytes) is canonical; Line and
// Col are 1-based, with Col counted in runes (combining diacritical
// marks do not advance it).
type Pos struct {
	Offset int
	Line   int
	Col    int
}

func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

// Span is a half-open source range [Start, End).
type Span struct {
	Start Pos
	End   Pos
}
