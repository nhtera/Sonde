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

	"github.com/nhtera/sonde/internal/syntax"
)

// parsedFile is an input file read and parsed.
type parsedFile struct {
	name string
	src  []byte
	file *syntax.File
}

// readInput reads and parses a request file. Problems are printed to stderr
// in the same form as run errors; the returned code is ExitOK or ExitParse.
func readInput(stderr io.Writer, name string) (*parsedFile, int) {
	src, err := readLimited(name)
	if err != nil {
		reportReadError(stderr, name, err)
		return nil, ExitParse
	}
	f, err := syntax.Parse(name, src, syntax.DialectFor(name))
	if err != nil {
		var perr *syntax.Error
		if errors.As(err, &perr) && perr.Kind == syntax.ErrInvalidUTF8 {
			reportReadError(stderr, name, utf8Error(src))
		} else if errors.As(err, &perr) {
			// Positions skip a byte order mark; so does the displayed line.
			shown := bytes.TrimPrefix(src, []byte("\uFEFF"))
			_, _ = fmt.Fprintf(stderr, "error: %s\n\n", perr.Render(name, shown))
		} else {
			reportReadError(stderr, name, err)
		}
		return nil, ExitParse
	}
	return &parsedFile{name: name, src: src, file: f}, ExitOK
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
