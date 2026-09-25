// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package jsonpath

import (
	"testing"

	"github.com/nhtera/sonde/internal/value"
)

func TestUnwrap(t *testing.T) {
	tests := []struct {
		name   string
		nodes  []value.Value
		want   value.Value
		wantOK bool
	}{
		{"zero nodes", nil, nil, false},
		{"empty slice", []value.Value{}, nil, false},
		{"one node", []value.Value{value.Int(1)}, value.Int(1), true},
		{
			"many nodes",
			[]value.Value{value.Int(1), value.Int(2)},
			value.List{value.Int(1), value.Int(2)},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Unwrap(tt.nodes)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && !value.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
