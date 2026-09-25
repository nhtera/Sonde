// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package value

import (
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
)

// DecodeJSON parses a complete JSON document. Numbers are classified by
// NumberFromText without precision loss; object members are sorted by key
// (byte-wise) and a duplicated key keeps its last value.
func DecodeJSON(text string) (Value, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid character after top-level value")
	}
	return FromJSON(raw), nil
}

// FromJSON converts a value produced by encoding/json (decoded with
// UseNumber) into a Value.
func FromJSON(raw any) Value {
	switch v := raw.(type) {
	case nil:
		return Null{}
	case bool:
		return Bool(v)
	case json.Number:
		return NumberFromText(string(v))
	case float64:
		return Float(v)
	case string:
		return String(v)
	case []any:
		list := make(List, len(v))
		for i, e := range v {
			list[i] = FromJSON(e)
		}
		return list
	case map[string]any:
		obj := make(Object, 0, len(v))
		for k, e := range v {
			obj = append(obj, Member{Key: k, Value: FromJSON(e)})
		}
		slices.SortFunc(obj, func(a, b Member) int { return strings.Compare(a.Key, b.Key) })
		return obj
	}
	return Null{}
}
