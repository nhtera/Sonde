// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"crypto/md5" //nolint:gosec // G501: MD5 is the Digest scheme's default algorithm
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"strings"
)

// Digest is a Digest challenge (RFC 7616) from a WWW-Authenticate header.
type Digest struct {
	Realm, Nonce, Opaque, Algorithm string
	// QOP is the protection chosen among the offered ones: "auth",
	// "auth-int", or "" for none (RFC 2069).
	QOP string
	// UserHash asks for the hashed user name (RFC 7616).
	UserHash bool
}

// ParseDigest reads the Digest challenge among WWW-Authenticate values.
func ParseDigest(values []string) (*Digest, error) {
	for _, v := range values {
		params, ok := challengeParams(v, "Digest")
		if !ok {
			continue
		}
		d := &Digest{Realm: params["realm"], Nonce: params["nonce"], Opaque: params["opaque"], Algorithm: params["algorithm"]}
		if d.Nonce == "" {
			return nil, errors.New("digest: challenge without a nonce")
		}
		switch strings.ToUpper(strings.TrimSuffix(strings.ToUpper(d.Algorithm), "-SESS")) {
		case "", "MD5", "SHA-256":
		default:
			return nil, fmt.Errorf("digest: unsupported algorithm %q", d.Algorithm)
		}
		for q := range strings.SplitSeq(params["qop"], ",") {
			switch strings.TrimSpace(q) {
			case "auth":
				d.QOP = "auth"
			case "auth-int":
				if d.QOP == "" {
					d.QOP = "auth-int"
				}
			}
		}
		d.UserHash = strings.EqualFold(params["userhash"], "true")
		return d, nil
	}
	return nil, errors.New("digest: no Digest challenge")
}

// NewCnonce returns a random client nonce.
func NewCnonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Authorization returns the Authorization header value answering the
// challenge for a request (method, request-target uri, body), with client
// nonce cnonce and nonce count nc.
func (d *Digest) Authorization(user, password, method, uri string, body []byte, cnonce string, nc int) string {
	h := d.hash
	ha1 := h(user + ":" + d.Realm + ":" + password)
	if strings.HasSuffix(strings.ToUpper(d.Algorithm), "-SESS") {
		ha1 = h(ha1 + ":" + d.Nonce + ":" + cnonce)
	}
	ha2 := h(method + ":" + uri)
	if d.QOP == "auth-int" {
		ha2 = h(method + ":" + uri + ":" + h(string(body)))
	}
	ncs := fmt.Sprintf("%08x", nc)
	var response string
	if d.QOP == "" {
		response = h(ha1 + ":" + d.Nonce + ":" + ha2)
	} else {
		response = h(ha1 + ":" + d.Nonce + ":" + ncs + ":" + cnonce + ":" + d.QOP + ":" + ha2)
	}
	name := user
	if d.UserHash {
		name = h(user + ":" + d.Realm)
	}
	var b strings.Builder
	fmt.Fprintf(&b, `Digest username="%s", realm="%s", nonce="%s", uri="%s"`, quote(name), quote(d.Realm), quote(d.Nonce), quote(uri))
	if d.QOP != "" {
		fmt.Fprintf(&b, `, cnonce="%s", nc=%s, qop=%s`, quote(cnonce), ncs, d.QOP)
	}
	fmt.Fprintf(&b, `, response="%s"`, response)
	if d.Opaque != "" {
		fmt.Fprintf(&b, `, opaque="%s"`, quote(d.Opaque))
	}
	if d.Algorithm != "" {
		fmt.Fprintf(&b, `, algorithm=%s`, d.Algorithm)
	}
	if d.UserHash {
		b.WriteString(`, userhash=true`)
	}
	return b.String()
}

// hash is the challenge's algorithm, hex encoded.
func (d *Digest) hash(s string) string {
	var h hash.Hash
	if strings.HasPrefix(strings.ToUpper(d.Algorithm), "SHA-256") {
		h = sha256.New()
	} else {
		h = md5.New() //nolint:gosec // G401: required by the Digest scheme
	}
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// quote escapes a quoted-string value.
func quote(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// challengeParams parses "Scheme k=v, k2="v 2"" when it is scheme's
// challenge; names are lower-case.
func challengeParams(v, scheme string) (map[string]string, bool) {
	v = strings.TrimSpace(v)
	// The challenge may follow others in one header: "Basic realm=x,
	// Digest ...".
	start := -1
	lower, want := strings.ToLower(v), strings.ToLower(scheme)
	for i := 0; i+len(want) <= len(lower); i++ {
		if lower[i:i+len(want)] == want && (i == 0 || lower[i-1] == ' ' || lower[i-1] == ',') &&
			(i+len(want) == len(lower) || lower[i+len(want)] == ' ') {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, false
	}
	v = v[start:]
	params := map[string]string{}
	rest := v[len(scheme):]
	for {
		rest = strings.TrimLeft(rest, " \t,")
		if rest == "" {
			return params, true
		}
		eq := strings.IndexByte(rest, '=')
		if eq < 0 {
			return params, true
		}
		name := strings.ToLower(strings.TrimSpace(rest[:eq]))
		rest = strings.TrimLeft(rest[eq+1:], " \t")
		var value string
		if strings.HasPrefix(rest, `"`) {
			var b strings.Builder
			i := 1
			for ; i < len(rest) && rest[i] != '"'; i++ {
				if rest[i] == '\\' && i+1 < len(rest) {
					i++
				}
				b.WriteByte(rest[i])
			}
			value, rest = b.String(), rest[min(i+1, len(rest)):]
		} else {
			end := strings.IndexByte(rest, ',')
			if end < 0 {
				end = len(rest)
			}
			value, rest = strings.TrimSpace(rest[:end]), rest[end:]
		}
		params[name] = value
	}
}
