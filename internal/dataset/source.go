// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package dataset streams the rows of a data file (`--data`): a CSV file
// with a header row, or a JSON array of objects. Rows are read one at a
// time, so memory does not grow with the number of rows.
package dataset

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Field is one cell of a row.
type Field struct {
	Name string
	// Raw is the cell text (CSV) or the JSON text of the value (JSON).
	Raw string
	// JSON reports whether Raw is JSON text.
	JSON bool
}

// Row is one data row.
type Row struct {
	// Index is the 1-based position of the row in the file.
	Index int
	// Line is the line the row starts on (CSV only; 0 for JSON).
	Line int
	// Fields are the row's cells, in column order (CSV) or sorted by name
	// (JSON).
	Fields []Field
}

// Reader reads the rows of a data file.
type Reader interface {
	// Next returns the next row, or io.EOF after the last one.
	Next() (Row, error)
	Close() error
}

// Open opens a data file, its format chosen by its extension (.csv or
// .json).
func Open(path string) (Reader, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".csv" && ext != ".json" {
		return nil, fmt.Errorf("%s: unsupported data file format (expected .csv or .json)", path)
	}
	f, err := os.Open(path) //nolint:gosec // G304: the data file is named on the command line
	if err != nil {
		return nil, err
	}
	var r Reader
	if ext == ".csv" {
		r, err = newCSVReader(path, f)
	} else {
		r, err = newJSONReader(path, f)
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return r, nil
}

// Each calls fn for every row of the data file at path, stopping at the
// first error (fn's or the file's).
func Each(path string, fn func(Row) error) error {
	r, err := Open(path)
	if err != nil {
		return err
	}
	defer r.Close() //nolint:errcheck // read-only
	for {
		row, err := r.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := fn(row); err != nil {
			return err
		}
	}
}
