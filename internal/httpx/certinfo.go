// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/nhtera/sonde/exchange"
)

// certInfo builds an exchange.CertInfo from the leaf certificate of a TLS
// connection state, formatting Subject and Issuer the way libcurl's OpenSSL
// backend does: RDNs in certificate order, "Short = value" joined by "; ".
func certInfo(state tls.ConnectionState) *exchange.CertInfo {
	if len(state.PeerCertificates) == 0 {
		return nil
	}
	cert := state.PeerCertificates[0]
	return &exchange.CertInfo{
		Subject:        formatRDN(cert.Subject),
		Issuer:         formatRDN(cert.Issuer),
		StartDate:      cert.NotBefore,
		ExpireDate:     cert.NotAfter,
		SerialNumber:   formatSerial(cert),
		SubjectAltName: formatSAN(cert),
		Value:          string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})),
	}
}

// formatRDN renders name's relative distinguished names in the order they
// appear in the certificate, e.g. "C = US, O = Let's Encrypt, CN = R1".
func formatRDN(name pkix.Name) string {
	parts := make([]string, 0, len(name.Names))
	for _, atv := range name.Names {
		short, ok := oidShortName[atv.Type.String()]
		if !ok {
			continue
		}
		val := fmt.Sprintf("%v", atv.Value)
		parts = append(parts, short+" = "+val)
	}
	return strings.Join(parts, ", ")
}

// oidShortName maps the attribute OIDs pkix.Name recognizes to their short
// names, as used in a distinguished name string.
var oidShortName = map[string]string{
	"2.5.4.3":                    "CN",
	"2.5.4.4":                    "SN",
	"2.5.4.5":                    "SERIALNUMBER",
	"2.5.4.6":                    "C",
	"2.5.4.7":                    "L",
	"2.5.4.8":                    "ST",
	"2.5.4.9":                    "STREET",
	"2.5.4.10":                   "O",
	"2.5.4.11":                   "OU",
	"2.5.4.17":                   "postalCode",
	"2.5.4.42":                   "GN",
	"1.2.840.113549.1.9.1":       "emailAddress",
	"0.9.2342.19200300.100.1.25": "DC",
}

// formatSerial renders the certificate serial number as libcurl does:
// lowercase hex bytes separated by colons.
func formatSerial(cert *x509.Certificate) string {
	b := cert.SerialNumber.Bytes()
	parts := make([]string, len(b))
	for i, v := range b {
		parts[i] = fmt.Sprintf("%02x", v)
	}
	return strings.Join(parts, ":")
}

// formatSAN renders the certificate's subject alternative names, one per
// line as "DNS:name" / "IP Address:addr", matching libcurl's rendering.
func formatSAN(cert *x509.Certificate) string {
	var lines []string
	for _, d := range cert.DNSNames {
		lines = append(lines, "DNS:"+d)
	}
	for _, ip := range cert.IPAddresses {
		lines = append(lines, "IP Address:"+ip.String())
	}
	for _, u := range cert.URIs {
		lines = append(lines, "URI:"+u.String())
	}
	for _, e := range cert.EmailAddresses {
		lines = append(lines, "email:"+e)
	}
	return strings.Join(lines, ", ")
}
