// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/sonde/exchange"
)

// TestSigV4Vectors checks signatures against the AWS Signature Version 4
// test suite (get-vanilla, get-vanilla-query-order-key-case).
func TestSigV4Vectors(t *testing.T) {
	s := SigV4{Provider1: "aws", Provider2: "amz", Region: "us-east-1", Service: "service"}
	at, _ := time.Parse("20060102T150405Z", "20150830T123600Z")
	for target, want := range map[string]string{
		"/":                             "5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31",
		"/?Param2=value2&Param1=value1": "b97d918cfa904a5beff61c982a1b6f458b799221646efd99d3219ec94cdf2500",
	} {
		u, _ := url.Parse("https://example.amazonaws.com" + target)
		//nolint:gosec // G101: the AWS test suite's documented example key
		add := s.Sign(SigV4Request{Method: "GET", URL: u, Host: "example.amazonaws.com", AccessKey: "AKIDEXAMPLE",
			SecretKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", Time: at})
		auth := add[len(add)-1].Value
		if !strings.HasSuffix(auth, "Signature="+want) {
			t.Errorf("%s: %s", target, auth)
		}
		if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;x-amz-date, ") {
			t.Errorf("%s: %s", target, auth)
		}
		if add[0] != (exchange.Header{Name: "X-Amz-Date", Value: "20150830T123600Z"}) {
			t.Errorf("date header %v", add[0])
		}
	}
}

func TestParseSigV4(t *testing.T) {
	for spec, want := range map[string]SigV4{
		"aws:amz:eu-central-1:hurltest": {"aws", "amz", "eu-central-1", "hurltest"},
		"aws:amz":                       {"aws", "amz", "eu-west-3", "s3"},
		"aws":                           {"aws", "aws", "eu-west-3", "s3"},
		"osc:amz::api":                  {"osc", "amz", "eu-west-3", "api"},
	} {
		got, err := ParseSigV4(spec, "s3.eu-west-3.amazonaws.com")
		if err != nil || got != want {
			t.Errorf("%s: %+v, %v", spec, got, err)
		}
	}
	if _, err := ParseSigV4("aws:amz", "localhost"); err == nil {
		t.Error("no region or service inferred from localhost")
	}
}

// TestDigestVectors checks responses against RFC 2617 (MD5) and RFC 7616
// (MD5 and SHA-256) examples.
func TestDigestVectors(t *testing.T) {
	d, err := ParseDigest([]string{`Basic realm="x"`, `Digest realm="testrealm@host.com", qop="auth,auth-int", nonce="dcd98b7102dd2f0e8b11d0f600bfb0c093", opaque="5ccc069c403ebaf9f0171e9517f40e41"`})
	if err != nil {
		t.Fatal(err)
	}
	got := d.Authorization("Mufasa", "Circle Of Life", "GET", "/dir/index.html", nil, "0a4f113b", 1)
	want := `Digest username="Mufasa", realm="testrealm@host.com", nonce="dcd98b7102dd2f0e8b11d0f600bfb0c093", uri="/dir/index.html", cnonce="0a4f113b", nc=00000001, qop=auth, response="6629fae49393a05397450978507c4ef1", opaque="5ccc069c403ebaf9f0171e9517f40e41"`
	if got != want {
		t.Errorf("RFC 2617:\n%s\nwant\n%s", got, want)
	}
	for algorithm, response := range map[string]string{
		"MD5":     "8ca523f5e9506fed4657c9700eebdbec",
		"SHA-256": "753927fa0e85d155564e2e272a28d1802ca10daf4496794697cf8db5856cb6c1",
	} {
		d, err := ParseDigest([]string{`Digest realm="http-auth@example.org", qop="auth, auth-int", algorithm=` + algorithm +
			`, nonce="7ypf/xlj9XXwfDPEoM4URrv/xwf94BcCAzFZH4GiTo0v", opaque="FQhe/qaU925kfnzjCev0ciny7QMkPqMAFRtzCUYo5tdS"`})
		if err != nil {
			t.Fatal(err)
		}
		got := d.Authorization("Mufasa", "Circle of Life", "GET", "/dir/index.html", nil, "f2/wE4q74E6zIJEtWaHKaf5wv/H5QzzpXusqGemxURZJ", 1)
		if !strings.Contains(got, `response="`+response+`"`) || !strings.HasSuffix(got, "algorithm="+algorithm) {
			t.Errorf("RFC 7616 %s: %s", algorithm, got)
		}
	}
	if _, err := ParseDigest([]string{`Digest realm="r"`}); err == nil {
		t.Error("a challenge without a nonce is accepted")
	}
	if _, err := ParseDigest([]string{`Digest nonce="n", algorithm=SHA-512-256`}); err == nil {
		t.Error("an unsupported algorithm is accepted")
	}
}

// TestNTLM checks the Type-1 message and a Type-3 answer to a challenge
// (the one the reference's test server sends), empty credentials
// included.
func TestNTLM(t *testing.T) {
	neg, err := NTLMNegotiate()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(neg, "NTLM "))
	if !strings.HasPrefix(string(raw), "NTLMSSP\x00\x01") {
		t.Errorf("Type-1 %q", raw)
	}
	challenge := "NTLM TlRMTVNTUAACAAAAAwAMADgAAAAzgoriASNFZ4mrze8AAAAAAAAAACQAJABEAAAABgBwFwAAAA9TAGUAcgB2AGUAcgACAAwARABvAG0AYQBpAG4AAQAMAFMAZQByAHYAZQByAAAAAAA="
	for _, cred := range [][2]string{{`domain\username`, "password"}, {"", ""}} {
		auth, err := NTLMAuthenticate([]string{"Negotiate", challenge}, cred[0], cred[1])
		if err != nil {
			t.Fatalf("%q: %v", cred[0], err)
		}
		raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "NTLM "))
		if !strings.HasPrefix(string(raw), "NTLMSSP\x00\x03") {
			t.Errorf("Type-3 %q", raw)
		}
	}
	if _, err := NTLMAuthenticate([]string{"NTLM"}, "u", "p"); err == nil {
		t.Error("no challenge accepted")
	}
}

// TestNegotiateWithoutTicket checks the error without a credential cache.
func TestNegotiateWithoutTicket(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KRB5_CONFIG", dir+"/krb5.conf")
	if _, err := NewNegotiator(); err == nil {
		t.Error("no error without configuration")
	}
	if err := os.WriteFile(dir+"/krb5.conf", []byte("[libdefaults]\n default_realm = EXAMPLE.TEST\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KRB5CCNAME", "FILE:"+dir+"/missing")
	if _, err := NewNegotiator(); err == nil || !strings.Contains(err.Error(), "no Kerberos ticket") {
		t.Errorf("error %v", err)
	}
}

// TestNTLMv2Vector checks NTOWFv2 and NTProofStr against MS-NLMP 4.2.4
// (User/Domain/Password, server challenge 0123456789abcdef, client
// challenge aa..., time 0).
func TestNTLMv2Vector(t *testing.T) {
	hash := ntlmV2Hash("User", "Domain", "Password")
	if got := hex.EncodeToString(hash); got != "0c868a403bfd7a93a3001ef22ef02e3f" {
		t.Errorf("NTOWFv2 %s", got)
	}
	targetInfo, _ := hex.DecodeString("02000c0044006f006d00610069006e0001000c0053006500720076006500720000000000")
	server, _ := hex.DecodeString("0123456789abcdef")
	client, _ := hex.DecodeString("aaaaaaaaaaaaaaaa")
	resp := ntlmV2Response(hash, server, client, make([]byte, 8), targetInfo)
	if got := hex.EncodeToString(resp[:16]); got != "68cd0ab851e51c96aabc927bebef6a1c" {
		t.Errorf("NTProofStr %s", got)
	}
}

// TestNTLMType3Layout checks the Type-3 message's buffers: each offset
// and length points inside the message, the user and domain read back,
// and the key-exchange flag is not echoed.
func TestNTLMType3Layout(t *testing.T) {
	raw, _ := base64.StdEncoding.DecodeString("TlRMTVNTUAACAAAAAwAMADgAAAAzgoriASNFZ4mrze8AAAAAAAAAACQAJABEAAAABgBwFwAAAA9TAGUAcgB2AGUAcgACAAwARABvAG0AYQBpAG4AAQAMAFMAZQByAHYAZQByAAAAAAA=")
	c, err := parseNTLMChallenge(raw)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := ntlmAuthenticateMessage(c, `DOM\bob`, "pw", make([]byte, 8), time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	field := func(i int) []byte {
		pos := 12 + 8*i
		n := int(binary.LittleEndian.Uint16(msg[pos:]))
		off := int(binary.LittleEndian.Uint32(msg[pos+4:]))
		if off+n > len(msg) {
			t.Fatalf("field %d out of the message", i)
		}
		return msg[off : off+n]
	}
	if string(field(2)) != string(utf16le("DOM")) || string(field(3)) != string(utf16le("bob")) {
		t.Errorf("domain %q, user %q", field(2), field(3))
	}
	if len(field(1)) <= 16 || len(field(0)) != 24 {
		t.Errorf("NT response %d bytes, LM %d", len(field(1)), len(field(0)))
	}
	if binary.LittleEndian.Uint32(msg[60:])&ntlmKeyExchange != 0 {
		t.Error("key exchange echoed")
	}
	for _, bad := range []string{"", "TlRMTVNTUAA=", base64.StdEncoding.EncodeToString(raw[:20])} {
		if _, err := NTLMAuthenticate([]string{"NTLM " + bad}, "u", "p"); err == nil {
			t.Errorf("challenge %q accepted", bad)
		}
	}
}

// TestSigV4PathAndDate checks the canonical path (encoded twice except for
// S3) and that a request's own date header is used.
func TestSigV4PathAndDate(t *testing.T) {
	u, _ := url.Parse("https://h/a%20b/(c)")
	if got := canonicalPath(u, false); got != "/a%2520b/%28c%29" {
		t.Errorf("non-S3 path %s", got)
	}
	if got := canonicalPath(u, true); got != "/a%20b/%28c%29" {
		t.Errorf("S3 path %s", got)
	}
	s := SigV4{Provider1: "aws", Provider2: "amz", Region: "r", Service: "s"}
	add := s.Sign(SigV4Request{Method: "GET", URL: u, Host: "h", Headers: []exchange.Header{{Name: "x-amz-date", Value: "20200101T000000Z"}},
		AccessKey: "a", SecretKey: "b", Time: time.Now()})
	if len(add) != 1 || !strings.Contains(add[0].Value, "/20200101/r/s/aws4_request") {
		t.Errorf("headers %v", add)
	}
}

// TestDigestAmongChallenges checks a Digest challenge after another in
// one header.
func TestDigestAmongChallenges(t *testing.T) {
	d, err := ParseDigest([]string{`Basic realm="a", Digest realm="b", nonce="n", qop="auth"`})
	if err != nil || d.Realm != "b" || d.Nonce != "n" {
		t.Errorf("%+v, %v", d, err)
	}
}
