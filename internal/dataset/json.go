// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package dataset

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

// jsonReader reads a JSON array of objects, one element at a time.
type jsonReader struct {
	path  string
	f     *os.File
	dec   *json.Decoder
	index int
	done  bool
}

func newJSONReader(path string, f *os.File) (*jsonReader, error) {
	dec := json.NewDecoder(bufio.NewReader(f))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return nil, jsonError(path, 0, err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return nil, fmt.Errorf("%s: expected a JSON array of objects", path)
	}
	return &jsonReader{path: path, f: f, dec: dec}, nil
}

func (j *jsonReader) Next() (Row, error) {
	if j.done {
		return Row{}, io.EOF
	}
	if !j.dec.More() {
		if _, err := j.dec.Token(); err != nil { // the closing ']'
			return Row{}, jsonError(j.path, j.index+1, err)
		}
		if _, err := j.dec.Token(); err != io.EOF { //nolint:errorlint // io.EOF is returned unwrapped
			return Row{}, fmt.Errorf("%s: unexpected data after the array", j.path)
		}
		j.done = true
		return Row{}, io.EOF
	}
	index := j.index + 1
	var obj map[string]json.RawMessage
	if err := j.dec.Decode(&obj); err != nil {
		return Row{}, jsonError(j.path, index, err)
	}
	if obj == nil {
		return Row{}, fmt.Errorf("%s: row %d: expected an object", j.path, index)
	}
	fields := make([]Field, 0, len(obj))
	for name, raw := range obj {
		if strings.TrimSpace(name) == "" {
			return Row{}, fmt.Errorf("%s: row %d: blank column name", j.path, index)
		}
		fields = append(fields, Field{Name: name, Raw: string(raw), JSON: true})
	}
	slices.SortFunc(fields, func(a, b Field) int { return strings.Compare(a.Name, b.Name) })
	j.index = index
	return Row{Index: index, Fields: fields}, nil
}

func (j *jsonReader) Close() error { return j.f.Close() }

// jsonError describes a decoding error of row (0: before the first row).
func jsonError(path string, row int, err error) error {
	if _, ok := err.(*json.UnmarshalTypeError); ok { //nolint:errorlint // returned unwrapped by Decode
		return fmt.Errorf("%s: row %d: expected an object", path, row)
	}
	if err == io.EOF || err == io.ErrUnexpectedEOF { //nolint:errorlint // returned unwrapped by Decode
		return fmt.Errorf("%s: unexpected end of JSON", path)
	}
	if row == 0 {
		return fmt.Errorf("%s: %w", path, err)
	}
	return fmt.Errorf("%s: row %d: %w", path, row, err)
}
