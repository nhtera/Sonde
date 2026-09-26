// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		in   string
		unit DurationUnit
		want time.Duration
	}{
		{"10", Millisecond, 10 * time.Millisecond},
		{"10s", Millisecond, 10 * time.Second},
		{"10000ms", Second, 10000 * time.Millisecond},
		{"5m", Second, 5 * time.Minute},
		{"3h", Millisecond, 3 * time.Hour},
		{"0", Second, 0},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseDuration(tt.in, tt.unit)
			if err != nil {
				t.Fatalf("ParseDuration(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseDurationErrors(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "Invalid duration"},
		{"s", "Invalid duration"},
		{"10s10", "Invalid duration"},
		{"10mm", "Invalid duration unit mm"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			_, err := ParseDuration(tt.in, Millisecond)
			if err == nil {
				t.Fatalf("ParseDuration(%q): expected an error", tt.in)
			}
			if err.Error() != tt.want {
				t.Errorf("ParseDuration(%q) error = %q, want %q", tt.in, err.Error(), tt.want)
			}
		})
	}
}

func TestParseDurationOverflow(t *testing.T) {
	// A huge hour count overflows even a 64-bit millisecond count.
	if _, err := ParseDuration("99999999999999999999h", Millisecond); err == nil {
		t.Fatal("expected an overflow error")
	}
}

func TestDurationUnitString(t *testing.T) {
	tests := map[DurationUnit]string{Millisecond: "ms", Second: "s", Minute: "m", Hour: "h", DurationUnit(99): "?"}
	for unit, want := range tests {
		if got := unit.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", unit, got, want)
		}
	}
}
