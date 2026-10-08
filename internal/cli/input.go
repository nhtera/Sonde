// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"github.com/nhtera/sonde/internal/runplan"
	"github.com/nhtera/sonde/internal/syntax"
)

// parsedFile is an input file read and parsed.
type parsedFile struct {
	name string
	src  []byte
	file *syntax.File
}

// stdinName names standard input as an input file.
const stdinName = "-"

// utf8BOM is the UTF-8 byte order mark.
const utf8BOM = "\uFEFF"

// readInput reads and parses a request file. Problems are printed to stderr
// in the reference formatter's form; the returned code is ExitOK or
// ExitParse.
func readInput(stderr io.Writer, name string) (*parsedFile, int) {
	return readInputColor(stderr, name, false, false)
}

// readInputColor is readInput with errors coloured when color is set, and
// standard input read for stdinName when stdin is set.
func readInputColor(stderr io.Writer, name string, color, stdin bool) (*parsedFile, int) {
	var src []byte
	var err error
	if stdin && name == stdinName {
		src, err = runplan.ReadLimited(os.Stdin)
	} else {
		src, err = readLimited(name)
	}
	if err != nil {
		reportInputError(stderr, name, err, color)
		return nil, ExitParse
	}
	f, err := syntax.Parse(name, src, syntax.DialectFor(name))
	if err != nil {
		var perr *syntax.Error
		if errors.As(err, &perr) && perr.Kind == syntax.ErrInvalidUTF8 {
			reportInputError(stderr, name, utf8Error(src), color)
		} else if errors.As(err, &perr) {
			// Positions skip a byte order mark; so does the displayed line.
			shown := bytes.TrimPrefix(src, []byte(utf8BOM))
			if color {
				_, _ = fmt.Fprintf(stderr, "%serror%s: %s\n\n", ansiRedBold, ansiReset, perr.RenderColor(name, shown))
			} else {
				_, _ = fmt.Fprintf(stderr, "error: %s\n\n", perr.Render(name, shown))
			}
		} else {
			reportInputError(stderr, name, err, color)
		}
		return nil, ExitParse
	}
	return &parsedFile{name: name, src: src, file: f}, ExitOK
}

// reportInputError reports an input file that cannot be read, in the
// reference formatter's wording.
func reportInputError(stderr io.Writer, name string, err error, color bool) {
	writeErrorMessage(stderr, fmt.Sprintf("Input file %s can not be read - %v", name, err), color)
}

// writeErrorMessage prints "error: message", the label red and the message
// bold when color is set.
func writeErrorMessage(stderr io.Writer, message string, color bool) {
	if color {
		_, _ = fmt.Fprintf(stderr, "%serror%s: %s%s%s\n", ansiRedBold, ansiReset, ansiBold, message, ansiReset)
		return
	}
	_, _ = fmt.Fprintf(stderr, "error: %s\n", message)
}

// readLimited reads a file but stops past syntax.MaxFileSize, so devices,
// pipes or huge files cannot exhaust memory.
func readLimited(name string) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	src, err := io.ReadAll(io.LimitReader(f, syntax.MaxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(src) > syntax.MaxFileSize {
		return nil, fmt.Errorf("file is larger than %d MiB", syntax.MaxFileSize>>20)
	}
	return src, nil
}

func reportReadError(stderr io.Writer, name string, err error) {
	_, _ = fmt.Fprintf(stderr, "error: Issue reading from %s: %v\n", name, err)
}

// utf8Error describes the first invalid UTF-8 sequence in src.
func utf8Error(src []byte) error {
	for i := 0; i < len(src); {
		r, size := utf8.DecodeRune(src[i:])
		if r == utf8.RuneError && size <= 1 {
			if !utf8.FullRune(src[i:]) {
				return fmt.Errorf("incomplete utf-8 byte sequence from index %d", i)
			}
			return fmt.Errorf("invalid utf-8 sequence of 1 bytes from index %d", i)
		}
		i += size
	}
	return errors.New("invalid utf-8")
}
