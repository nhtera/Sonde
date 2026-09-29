// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package datarow reads a data file (--data) as engine rows: every row is
// checked once, then read again for each pass, so memory does not grow
// with the number of rows.
package datarow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/nhtera/sonde/engine"
	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/dataset"
	"github.com/nhtera/sonde/internal/value"
)

// RowVar is the built-in variable holding the 1-based row index.
const RowVar = "data_row"

// Run is a --data file, checked in full before the run: its rows are
// then read again, one at a time, for every input file and repeat, so
// memory does not grow with the number of rows.
type Run struct {
	path string
	// secret are the --data-secret columns.
	secret map[string]bool
	// overridden are the --variable names: they win over row values.
	overridden map[string]bool
	// secrets are the command line secrets: no column may take their
	// names.
	secrets map[string]string
	// columns are the column names of every row.
	columns map[string]bool
	// rows is the number of rows.
	rows int
}

// Open reads the data file at path once, checking every row, the
// --data-secret columns and name clashes with the command line secrets.
// overridden are the names of the --variable values, which win over row
// values. An empty path returns nil.
func Open(path string, secretCols, overridden []string, secrets map[string]string) (*Run, error) {
	d := &Run{path: path, secret: map[string]bool{}, overridden: map[string]bool{}, secrets: secrets, columns: map[string]bool{}}
	for _, col := range secretCols {
		col = strings.TrimSpace(col)
		if col == "" {
			return nil, errors.New("--data-secret: blank column name")
		}
		d.secret[col] = true
	}
	if len(d.secret) > 0 && path == "" {
		return nil, errors.New("--data-secret requires --data")
	}
	if path == "" {
		return nil, nil
	}
	if fi, err := os.Stat(path); err != nil {
		return nil, err
	} else if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", path)
	}
	for _, name := range overridden {
		d.overridden[name] = true
	}
	err := dataset.Each(path, func(r dataset.Row) error {
		for _, f := range r.Fields {
			d.columns[f.Name] = true
		}
		if _, err := d.row(r); err != nil {
			return err
		}
		d.rows++
		return nil
	})
	if err != nil {
		return nil, err
	}
	for col := range d.secret {
		if d.rows > 0 && !d.columns[col] {
			return nil, fmt.Errorf("%s: no column %q (--data-secret)", path, col)
		}
	}
	return d, nil
}

// CheckSecrets rejects a column named like one of secrets (a sonde.yaml
// environment's).
func (d *Run) CheckSecrets(secrets map[string]string) error {
	for name := range secrets {
		if d.columns[name] {
			return fmt.Errorf("data column %q is already defined as a secret", name)
		}
	}
	return nil
}

// Each calls yield with every row of the data file; it reports whether
// yield asked to stop, or the error of a data file that changed since it
// was checked.
func (d *Run) Each(yield func(*engine.Row) bool) (stopped bool, err error) {
	err = dataset.Each(d.path, func(r dataset.Row) error {
		row, err := d.row(r)
		if err != nil {
			return err
		}
		if !yield(row) {
			stopped = true
			return errStopRows
		}
		return nil
	})
	if errors.Is(err, errStopRows) {
		return true, nil
	}
	return stopped, err
}

var errStopRows = errors.New("stop")

// Rows is the number of rows.
func (d *Run) Rows() int { return d.rows }

// Columns returns the column names of every row, sorted.
func (d *Run) Columns() []string {
	names := make([]string, 0, len(d.columns))
	for name := range d.columns {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Path is the data file's path.
func (d *Run) Path() string { return d.path }

// row converts a data row: values are typed like --variable values (CSV)
// or kept as their JSON type; secret columns are text.
func (d *Run) row(r dataset.Row) (*engine.Row, error) {
	row := &engine.Row{Index: r.Index, Variables: make(map[string]any, len(r.Fields)+1)}
	if !d.overridden[RowVar] {
		row.Variables[RowVar] = value.Int(int64(r.Index))
	}
	for _, f := range r.Fields {
		_, isSecret := d.secrets[f.Name]
		switch {
		case f.Name == RowVar:
			return nil, fmt.Errorf("%s: column %q is reserved", rowPlace(d.path, r), RowVar)
		case isSecret:
			return nil, fmt.Errorf("%s: column %q is already defined as a secret", rowPlace(d.path, r), f.Name)
		case config.IsReserved(f.Name):
			return nil, fmt.Errorf("%s: column %q conflicts with the %s function, use a different name", rowPlace(d.path, r), f.Name, f.Name)
		case d.overridden[f.Name]:
			continue
		}
		if d.secret[f.Name] {
			s, ok, err := secretText(f)
			if err != nil {
				return nil, fmt.Errorf("%s: column %q: %w", rowPlace(d.path, r), f.Name, err)
			}
			if ok {
				if row.Secrets == nil {
					row.Secrets = map[string]string{}
				}
				row.Secrets[f.Name] = s
			}
			continue
		}
		var v value.Value
		var err error
		if f.JSON {
			v, err = value.DecodeJSON(f.Raw)
		} else {
			v, err = config.InferValue(f.Raw)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: column %q: %w", rowPlace(d.path, r), f.Name, err)
		}
		row.Variables[f.Name] = v
	}
	return row, nil
}

// secretText returns the text of a secret cell: a JSON string, number or
// boolean; a JSON null is no secret (ok false).
func secretText(f dataset.Field) (s string, ok bool, err error) {
	if !f.JSON {
		return f.Raw, true, nil
	}
	raw := strings.TrimSpace(f.Raw)
	switch {
	case raw == "null":
		return "", false, nil
	case strings.HasPrefix(raw, `"`):
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			return "", false, err
		}
		return s, true, nil
	case strings.HasPrefix(raw, "{"), strings.HasPrefix(raw, "["):
		return "", false, errors.New("a secret must be a string, a number or a boolean")
	}
	return raw, true, nil
}

// rowPlace locates a row in messages: its line (CSV) or its index (JSON).
func rowPlace(path string, r dataset.Row) string {
	if r.Line > 0 {
		return fmt.Sprintf("%s:%d", path, r.Line)
	}
	return fmt.Sprintf("%s: row %d", path, r.Index)
}
