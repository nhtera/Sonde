// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package dataset

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

// csvReader reads an RFC 4180 CSV file whose first row names the columns.
type csvReader struct {
	path   string
	f      *os.File
	r      *csv.Reader
	header []string
	index  int
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func newCSVReader(path string, f *os.File) (*csvReader, error) {
	br := bufio.NewReader(f)
	if b, err := br.Peek(len(utf8BOM)); err == nil && bytes.Equal(b, utf8BOM) {
		_, _ = br.Discard(len(utf8BOM))
	}
	r := csv.NewReader(br)
	r.ReuseRecord = true
	header, err := r.Read()
	if errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: missing header row", path)
	}
	if err != nil {
		return nil, csvError(path, err)
	}
	seen := make(map[string]bool, len(header))
	names := make([]string, len(header))
	for i, name := range header {
		name = strings.TrimSpace(name)
		switch {
		case name == "":
			return nil, fmt.Errorf("%s:1: column %d has a blank name", path, i+1)
		case seen[name]:
			return nil, fmt.Errorf("%s:1: duplicate column %q", path, name)
		}
		seen[name] = true
		names[i] = name
	}
	return &csvReader{path: path, f: f, r: r, header: names}, nil
}

func (c *csvReader) Next() (Row, error) {
	record, err := c.r.Read()
	if errors.Is(err, io.EOF) {
		return Row{}, io.EOF
	}
	if err != nil {
		return Row{}, csvError(c.path, err)
	}
	line, _ := c.r.FieldPos(0)
	fields := make([]Field, len(record))
	for i, cell := range record {
		if !utf8.ValidString(cell) {
			return Row{}, fmt.Errorf("%s:%d: column %q is not valid UTF-8", c.path, line, c.header[i])
		}
		fields[i] = Field{Name: c.header[i], Raw: cell}
	}
	c.index++
	return Row{Index: c.index, Line: line, Fields: fields}, nil
}

func (c *csvReader) Close() error { return c.f.Close() }

// csvError prefixes a CSV parse error with the file and its line.
func csvError(path string, err error) error {
	var pe *csv.ParseError
	if errors.As(err, &pe) {
		return fmt.Errorf("%s:%d: %w", path, pe.Line, pe.Err)
	}
	return fmt.Errorf("%s: %w", path, err)
}
