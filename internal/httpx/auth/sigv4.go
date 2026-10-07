// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package auth computes the Authorization headers of the HTTP
// authentication schemes beyond Basic: AWS Signature Version 4, Digest,
// NTLM and Negotiate (SPNEGO). It only builds header values; the client
// sends them and runs the challenge exchanges.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/nhtera/sonde/exchange"
)

// SigV4 is a parsed --aws-sigv4 value:
// provider1[:provider2[:region[:service]]], as curl reads it.
type SigV4 struct {
	Provider1, Provider2, Region, Service string
}

// ParseSigV4 reads spec; a missing region or service is taken from host
// (service.region.provider.tld, as curl infers them).
func ParseSigV4(spec, host string) (SigV4, error) {
	parts := strings.SplitN(spec, ":", 4)
	s := SigV4{Provider1: parts[0]}
	if s.Provider1 == "" {
		return s, errors.New("aws-sigv4: missing provider")
	}
	s.Provider2 = s.Provider1
	if len(parts) > 1 && parts[1] != "" {
		s.Provider2 = parts[1]
	}
	if len(parts) > 2 {
		s.Region = parts[2]
	}
	if len(parts) > 3 {
		s.Service = parts[3]
	}
	if s.Region == "" || s.Service == "" {
		labels := strings.Split(host, ".")
		if s.Service == "" && len(labels) > 0 {
			s.Service = labels[0]
		}
		if s.Region == "" && len(labels) > 1 {
			s.Region = labels[1]
		}
	}
	if s.Region == "" || s.Service == "" {
		return s, fmt.Errorf("aws-sigv4: cannot infer the region and service from host %q", host)
	}
	for _, p := range []string{s.Provider1, s.Provider2, s.Region, s.Service} {
		if strings.ContainsAny(p, " \t\r\n/") {
			return s, fmt.Errorf("aws-sigv4: invalid value %q", spec)
		}
	}
	return s, nil
}

// SigV4Request is what a signature covers.
type SigV4Request struct {
	Method string
	URL    *url.URL
	Host   string // the Host header sent
	// Headers are signed with Host and the date header, names lower-case.
	Headers []exchange.Header
	Body    []byte
	// AccessKey and SecretKey sign. A session token is a header of the
	// request (x-amz-security-token), signed with the others.
	AccessKey, SecretKey string
	Time                 time.Time
}

// Sign returns the headers to add to the request: the date (unless the
// request has its own x-<provider2>-date, which is used as is) and the
// content hash for S3, then Authorization.
func (s SigV4) Sign(r SigV4Request) []exchange.Header {
	p1, p2 := strings.ToLower(s.Provider1), strings.ToLower(s.Provider2)
	dateHeader := "X-" + title(p2) + "-Date"
	stamp := r.Time.UTC().Format("20060102T150405Z")
	var add []exchange.Header
	if v, ok := exchange.Headers(r.Headers).Get(dateHeader); ok && len(v) >= 8 {
		stamp = v
	} else {
		add = append(add, exchange.Header{Name: dateHeader, Value: stamp})
	}
	date := stamp[:8]
	payload := hexSHA256(r.Body)

	if s.Service == "s3" {
		add = append(add, exchange.Header{Name: "X-" + title(p2) + "-Content-Sha256", Value: payload})
	}

	signed := map[string][]string{"host": {r.Host}}
	for _, h := range append(slices.Clone(r.Headers), add...) {
		name := strings.ToLower(h.Name)
		signed[name] = append(signed[name], strings.Join(strings.Fields(h.Value), " "))
	}
	names := make([]string, 0, len(signed))
	for name := range signed {
		names = append(names, name)
	}
	slices.Sort(names)
	var canonHeaders strings.Builder
	for _, name := range names {
		canonHeaders.WriteString(name + ":" + strings.Join(signed[name], ",") + "\n")
	}
	signedHeaders := strings.Join(names, ";")

	canonical := strings.Join([]string{
		r.Method,
		canonicalPath(r.URL, s.Service == "s3"),
		canonicalQuery(r.URL.RawQuery),
		canonHeaders.String(),
		signedHeaders,
		payload,
	}, "\n")
	scope := date + "/" + s.Region + "/" + s.Service + "/" + p1 + "4_request"
	algorithm := strings.ToUpper(p1) + "4-HMAC-SHA256"
	toSign := algorithm + "\n" + stamp + "\n" + scope + "\n" + hexSHA256([]byte(canonical))

	key := hmacSHA256([]byte(strings.ToUpper(p1)+"4"+r.SecretKey), date)
	key = hmacSHA256(key, s.Region)
	key = hmacSHA256(key, s.Service)
	key = hmacSHA256(key, p1+"4_request")
	signature := hex.EncodeToString(hmacSHA256(key, toSign))

	auth := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s", algorithm, r.AccessKey, scope, signedHeaders, signature)
	return append(add, exchange.Header{Name: "Authorization", Value: auth})
}

// canonicalPath is the path URI-encoded once for S3 and twice (each
// segment of the path as sent encoded again) for other services, as the
// SigV4 specification and curl do; "/" when empty.
func canonicalPath(u *url.URL, s3 bool) string {
	p := u.EscapedPath()
	if p == "" {
		return "/"
	}
	segments := strings.Split(p, "/")
	for i, seg := range segments {
		if s3 {
			segments[i] = awsEscape(unescapePath(seg))
		} else {
			segments[i] = awsEscape(seg)
		}
	}
	return strings.Join(segments, "/")
}

func unescapePath(s string) string {
	if u, err := url.PathUnescape(s); err == nil {
		return u
	}
	return s
}

// canonicalQuery sorts the query parameters and encodes them as SigV4
// requires (unreserved characters only, %XX upper-case).
func canonicalQuery(raw string) string {
	if raw == "" {
		return ""
	}
	type param struct{ name, value string }
	var params []param
	for part := range strings.SplitSeq(raw, "&") {
		if part == "" {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		params = append(params, param{awsEscape(unescape(name)), awsEscape(unescape(value))})
	}
	slices.SortFunc(params, func(a, b param) int {
		if c := strings.Compare(a.name, b.name); c != 0 {
			return c
		}
		return strings.Compare(a.value, b.value)
	})
	out := make([]string, len(params))
	for i, p := range params {
		out[i] = p.name + "=" + p.value
	}
	return strings.Join(out, "&")
}

func unescape(s string) string {
	if u, err := url.QueryUnescape(s); err == nil {
		return u
	}
	return s
}

// awsEscape percent-encodes every byte but the unreserved characters.
func awsEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.IndexByte("-_.~", c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func hexSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}
