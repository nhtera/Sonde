// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"crypto/x509"
	"net"
	"testing"
)

func TestFormatSAN(t *testing.T) {
	cert := &x509.Certificate{
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1").To4(), net.ParseIP("::1"), net.ParseIP("2001:db8::ab:1"), net.ParseIP("::ffff:1.2.3.4")},
	}
	want := "DNS:localhost, IP Address:127.0.0.1, IP Address:0:0:0:0:0:0:0:1, IP Address:2001:DB8:0:0:0:0:AB:1, IP Address:0:0:0:0:0:FFFF:102:304"
	if got := formatSAN(cert); got != want {
		t.Errorf("formatSAN = %q\nwant %q", got, want)
	}
}
