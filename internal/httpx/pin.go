// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"strings"
)

// errPinMismatch reports that none of the pinned keys matched the peer's
// certificate chain.
var errPinMismatch = errors.New("SSL public key does not match pinned public key")

// pinnedKeyChecker parses --pinned-pubkey (`;`-separated `sha256//BASE64`
// hashes, or the path to a PEM/DER certificate or public key file) into a
// verifier used as tls.Config.VerifyConnection.
type pinnedKeyChecker struct {
	hashes [][32]byte
}

// parsePinnedPublicKey resolves spec into a checker; a file path is read
// with readFile. As with curl, a hash or file that cannot be used pins no
// key: the connection then fails as a mismatch.
func parsePinnedPublicKey(spec string, readFile func(string) ([]byte, error)) *pinnedKeyChecker {
	c := &pinnedKeyChecker{}
	for part := range strings.SplitSeq(spec, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if enc, ok := strings.CutPrefix(part, "sha256//"); ok {
			sum, err := base64.StdEncoding.DecodeString(enc)
			if err == nil && len(sum) == sha256.Size {
				c.hashes = append(c.hashes, [32]byte(sum))
			}
			continue
		}
		if data, err := readFile(part); err == nil {
			if h, err := spkiHashFromFile(data); err == nil {
				c.hashes = append(c.hashes, h)
			}
		}
	}
	return c
}

// spkiHashFromFile extracts the SHA-256 hash of the SubjectPublicKeyInfo
// from a PEM or DER encoded certificate or public key.
func spkiHashFromFile(data []byte) ([32]byte, error) {
	der := data
	if block, _ := pem.Decode(data); block != nil {
		der = block.Bytes
	}
	if cert, err := x509.ParseCertificate(der); err == nil {
		return sha256.Sum256(cert.RawSubjectPublicKeyInfo), nil
	}
	if pub, err := x509.ParsePKIXPublicKey(der); err == nil {
		spki, err := x509.MarshalPKIXPublicKey(pub)
		if err == nil {
			return sha256.Sum256(spki), nil
		}
	}
	return [32]byte{}, errors.New("unrecognized certificate or public key")
}

// verify checks the server's own certificate against the pinned hashes;
// the other certificates it sends prove nothing.
func (c *pinnedKeyChecker) verify(state tls.ConnectionState) error {
	if len(state.PeerCertificates) == 0 {
		return errPinMismatch
	}
	sum := sha256.Sum256(state.PeerCertificates[0].RawSubjectPublicKeyInfo)
	for _, want := range c.hashes {
		if sum == want {
			return nil
		}
	}
	return errPinMismatch
}
