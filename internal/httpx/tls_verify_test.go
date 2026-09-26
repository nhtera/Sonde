// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"testing"
	"time"
)

// issue creates a certificate from tmpl signed by parent (self-signed when
// parent is nil).
func issue(t *testing.T, tmpl *x509.Certificate, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl.SerialNumber = big.NewInt(time.Now().UnixNano())
	tmpl.NotBefore, tmpl.NotAfter = time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	if parent == nil {
		parent, parentKey = tmpl, key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

func testCA(t *testing.T, permitted ...string) (*x509.Certificate, *ecdsa.PrivateKey, *x509.CertPool) {
	t.Helper()
	ca, key := issue(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "test-ca"}, IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign, PermittedDNSDomains: permitted,
	}, nil, nil)
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return ca, key, pool
}

func leafCert(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, cn string, dns []string, ips []net.IP) *x509.Certificate {
	t.Helper()
	leaf, _ := issue(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: cn}, DNSNames: dns, IPAddresses: ips,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, ca, caKey)
	return leaf
}

func TestCurlVerifyHost(t *testing.T) {
	ca, caKey, pool := testCA(t)
	loopback := []net.IP{net.ParseIP("127.0.0.1")}
	for _, tc := range []struct {
		name string
		leaf *x509.Certificate
		host string
		ok   bool
	}{
		{"IP host, certificate for another name", leafCert(t, ca, caKey, "", []string{"evil.example"}, nil), "127.0.0.1", false},
		{"IP host, matching IP SAN", leafCert(t, ca, caKey, "", nil, loopback), "127.0.0.1", true},
		{"no host", leafCert(t, ca, caKey, "", nil, loopback), "", false},
		{"DNS SAN", leafCert(t, ca, caKey, "", []string{"example.test"}, nil), "example.test", true},
		{"Common Name only", leafCert(t, ca, caKey, "example.test", nil, nil), "example.test", true},
		{"Common Name mismatch", leafCert(t, ca, caKey, "example.test", nil, nil), "other.test", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := curlVerify(tls.ConnectionState{PeerCertificates: []*x509.Certificate{tc.leaf}}, pool, tc.host)
			if (err == nil) != tc.ok {
				t.Errorf("curlVerify = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

// A Common Name is not checked against name constraints, so it is not
// used when a certificate of the chain constrains names.
func TestCurlVerifyCommonNameUnderConstraints(t *testing.T) {
	ca, caKey, pool := testCA(t, "allowed.test")
	leaf := leafCert(t, ca, caKey, "outside.test", nil, nil)
	if err := curlVerify(tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}}, pool, "outside.test"); err == nil {
		t.Error("Common Name accepted under a name-constrained CA")
	}
}

// The pin applies to the server's own certificate, not to other
// certificates it sends along.
func TestPinnedKeyLeafOnly(t *testing.T) {
	ca, caKey, _ := testCA(t)
	genuine := leafCert(t, ca, caKey, "", []string{"example.test"}, nil)
	attacker := leafCert(t, ca, caKey, "", []string{"example.test"}, nil)
	c := &pinnedKeyChecker{hashes: [][32]byte{sha256.Sum256(genuine.RawSubjectPublicKeyInfo)}}
	if err := c.verify(tls.ConnectionState{PeerCertificates: []*x509.Certificate{attacker, genuine}}); !errors.Is(err, errPinMismatch) {
		t.Errorf("verify(attacker + appended genuine) = %v, want errPinMismatch", err)
	}
	if err := c.verify(tls.ConnectionState{PeerCertificates: []*x509.Certificate{genuine}}); err != nil {
		t.Errorf("verify(genuine) = %v", err)
	}
}
