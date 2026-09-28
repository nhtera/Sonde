// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package netpolicy

import (
	"errors"
	"testing"
)

func TestParseInvalid(t *testing.T) {
	for _, pat := range []string{
		"", "http://a.com", "a.com:0", "a.com:70000", "a.com:x", "*.*.com", "a*.com",
		"10.0.0.0/33", "a.com/8", "127.1", "0x7f000001", "*.1.2", "user@a.com", "[::1",
		"a%b.com", "exämple.com", "a..com", "a b.com", "*.",
	} {
		if _, err := Parse([]string{pat}); err == nil {
			t.Errorf("Parse(%q): want an error", pat)
		}
	}
}

// TestAllow is the table both enforcement points rely on: the URL check
// and the dial check call Allow with the host as written.
func TestAllow(t *testing.T) {
	p, err := Parse([]string{
		"api.example.com", "local.test:08443", "*.svc.example.org", "10.1.2.3",
		"[::1]:8080", "fd00::/8", "192.168.0.0/16", "Upper.Example.NET.",
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		host, port string
		ok         bool
	}{
		{"api.example.com", "443", true},
		{"API.Example.com.", "80", true},
		{"api.example.com.evil.com", "443", false},
		{"evil-api.example.com", "443", false},
		{"example.com", "443", false},
		{"local.test", "8443", true},
		{"local.test", "443", false},
		{"a.svc.example.org", "443", true},
		{"a.b.svc.example.org", "443", true},
		{"svc.example.org", "443", false},
		{"xsvc.example.org", "443", false},
		{"10.1.2.3", "80", true},
		{"10.1.2.4", "80", false},
		{"::ffff:10.1.2.3", "80", true},
		{"[::1]", "8080", true},
		{"::1", "8080", true},
		{"::1", "80", false},
		{"fe80::1%lo0", "80", false},
		{"fd12::5", "80", true},
		{"[fd12::5%eth0]", "80", true},
		{"192.168.4.4", "80", true},
		{"upper.example.net", "443", true},
		// Names never match address rules, even when they resolve there.
		{"localhost", "8080", false},
		// Short and hexadecimal IPv4 forms are refused outright.
		{"127.1", "80", false},
		{"0x7f000001", "80", false},
		{"0x7f.1", "80", false},
		{"", "80", false},
		// A '%' is cut by nothing but a zone of an IPv6 address: the
		// system resolver would look the whole name up.
		{"api.example.com%.127.0.0.1.nip.io", "443", false},
		{"api.example.com%25", "443", false},
		{"10.1.2.3%eth0", "80", false},
		// Only ASCII names: net/http dials the punycode form.
		{"api.exämple.com", "443", false},
		{"api..example.com", "443", false},
		{"local.test", "08443", true},
	}
	for _, tt := range tests {
		err := p.Allow(tt.host, tt.port)
		if (err == nil) != tt.ok {
			t.Errorf("Allow(%q, %q) = %v, want ok=%v", tt.host, tt.port, err, tt.ok)
		}
		if err != nil && !errors.Is(err, ErrDenied) {
			t.Errorf("Allow(%q, %q): error %v does not wrap ErrDenied", tt.host, tt.port, err)
		}
	}
}

func TestAllowAll(t *testing.T) {
	var nilPolicy *Policy
	if err := nilPolicy.Allow("anything", "1"); err != nil {
		t.Fatal(err)
	}
	p, err := Parse([]string{"a.com", "*"})
	if err != nil {
		t.Fatal(err)
	}
	if !p.AllowsAll() || p.Allow("b.com", "80") != nil {
		t.Fatal("* must allow every host")
	}
	empty, _ := Parse(nil)
	if empty.Allow("a.com", "80") == nil {
		t.Fatal("an empty allowlist must deny")
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"A.COM.":           "a.com",
		"[::1]":            "::1",
		"::FFFF:127.0.0.1": "127.0.0.1",
		"fe80::1%eth0":     "fe80::1",
		"cafe.de":          "cafe.de",
		"0xcafe.com":       "0xcafe.com",
	} {
		got, err := Normalize(in)
		if err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}
