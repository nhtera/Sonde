// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5" //nolint:gosec // G501: HMAC-MD5 is NTLMv2's algorithm
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"time"
	"unicode/utf16"

	"golang.org/x/crypto/md4" //nolint:staticcheck,gosec // SA1019, G506: MD4 is the NT hash NTLM is defined with
)

// NTLM negotiate flags (MS-NLMP 2.2.2.5).
const (
	ntlmUnicode          = 0x00000001
	ntlmOEM              = 0x00000002
	ntlmRequestTarget    = 0x00000004
	ntlmNTLM             = 0x00000200
	ntlmAlwaysSign       = 0x00008000
	ntlmExtendedSecurity = 0x00080000
	ntlmKeyExchange      = 0x40000000
)

// ntlmSignature starts every NTLM message.
const ntlmSignature = "NTLMSSP\x00"

// NTLMNegotiate returns the Authorization value that opens an NTLM
// exchange: the Type-1 message, with curl's flags and no domain or
// workstation.
func NTLMNegotiate() (string, error) {
	msg := make([]byte, 32)
	copy(msg, ntlmSignature)
	binary.LittleEndian.PutUint32(msg[8:], 1)
	binary.LittleEndian.PutUint32(msg[12:], ntlmUnicode|ntlmOEM|ntlmRequestTarget|ntlmNTLM|ntlmAlwaysSign|ntlmExtendedSecurity)
	// Empty domain and workstation buffers, both at the end of the message.
	binary.LittleEndian.PutUint32(msg[20:], 32)
	binary.LittleEndian.PutUint32(msg[28:], 32)
	return "NTLM " + base64.StdEncoding.EncodeToString(msg), nil
}

// ntlmChallenge is the part of a Type-2 message the answer needs.
type ntlmChallenge struct {
	flags      uint32
	challenge  []byte
	targetInfo []byte
}

func parseNTLMChallenge(msg []byte) (*ntlmChallenge, error) {
	if len(msg) < 32 || string(msg[:8]) != ntlmSignature || binary.LittleEndian.Uint32(msg[8:]) != 2 {
		return nil, errors.New("ntlm: invalid challenge")
	}
	c := &ntlmChallenge{flags: binary.LittleEndian.Uint32(msg[20:]), challenge: msg[24:32]}
	if len(msg) >= 48 {
		n := int(binary.LittleEndian.Uint16(msg[40:]))
		off := int(binary.LittleEndian.Uint32(msg[44:]))
		if n > 0 && off >= 48 && off+n <= len(msg) {
			c.targetInfo = msg[off : off+n]
		}
	}
	return c, nil
}

// NTLMAuthenticate answers the server's Type-2 challenge among the
// WWW-Authenticate values with an NTLMv2 Type-3 message, for user
// ("DOMAIN\user", or a user alone) and password; both may be empty, as
// with curl's -u ":". The target name is not read, and no session key
// is exchanged, as curl does.
func NTLMAuthenticate(values []string, user, password string) (string, error) {
	for _, v := range values {
		token, ok := strings.CutPrefix(strings.TrimSpace(v), "NTLM ")
		if !ok || strings.TrimSpace(token) == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(token))
		if err != nil {
			return "", errors.New("ntlm: invalid challenge")
		}
		c, err := parseNTLMChallenge(raw)
		if err != nil {
			return "", err
		}
		clientChallenge := make([]byte, 8)
		if _, err := rand.Read(clientChallenge); err != nil {
			return "", err
		}
		msg, err := ntlmAuthenticateMessage(c, user, password, clientChallenge, time.Now())
		if err != nil {
			return "", err
		}
		return "NTLM " + base64.StdEncoding.EncodeToString(msg), nil
	}
	return "", errors.New("ntlm: no NTLM challenge")
}

// ntlmAuthenticateMessage builds the Type-3 message (MS-NLMP 2.2.1.3).
func ntlmAuthenticateMessage(c *ntlmChallenge, user, password string, clientChallenge []byte, now time.Time) ([]byte, error) {
	domain := ""
	if i := strings.IndexAny(user, `\/`); i >= 0 {
		domain, user = user[:i], user[i+1:]
	}
	hash := ntlmV2Hash(user, domain, password)
	timestamp := avTimestamp(c.targetInfo, now)
	ntResponse := ntlmV2Response(hash, c.challenge, clientChallenge, timestamp, c.targetInfo)
	lmMAC := hmacMD5(hash, append(append([]byte{}, c.challenge...), clientChallenge...))
	lmResponse := append(lmMAC, clientChallenge...)

	flags := c.flags &^ ntlmKeyExchange
	encode := func(s string) []byte { return []byte(s) }
	if flags&ntlmUnicode != 0 {
		encode = utf16le
	}
	fields := [][]byte{lmResponse, ntResponse, encode(domain), encode(user), nil, nil} // workstation, session key empty
	for _, f := range fields {
		if len(f) > 0xffff {
			return nil, errors.New("ntlm: challenge target information too large")
		}
	}
	const header = 64
	msg := make([]byte, header)
	copy(msg, ntlmSignature)
	binary.LittleEndian.PutUint32(msg[8:], 3)
	off := header
	for i, f := range fields {
		pos := 12 + 8*i
		binary.LittleEndian.PutUint16(msg[pos:], uint16(len(f)))   //nolint:gosec // G115: checked above
		binary.LittleEndian.PutUint16(msg[pos+2:], uint16(len(f))) //nolint:gosec // G115: checked above
		binary.LittleEndian.PutUint32(msg[pos+4:], uint32(off))    //nolint:gosec // G115: the message is small
		off += len(f)
	}
	binary.LittleEndian.PutUint32(msg[60:], flags)
	for _, f := range fields {
		msg = append(msg, f...)
	}
	return msg, nil
}

// ntlmV2Hash is NTOWFv2: HMAC-MD5 keyed with the NT hash of password over
// the upper-case user and the domain.
func ntlmV2Hash(user, domain, password string) []byte {
	h := md4.New() //nolint:gosec // G406: the NT hash is MD4 by definition
	h.Write(utf16le(password))
	return hmacMD5(h.Sum(nil), utf16le(strings.ToUpper(user)+domain))
}

// ntlmV2Response is NTProofStr followed by the client blob.
func ntlmV2Response(hash, serverChallenge, clientChallenge, timestamp, targetInfo []byte) []byte {
	var blob bytes.Buffer
	blob.Write([]byte{1, 1, 0, 0, 0, 0, 0, 0})
	blob.Write(timestamp)
	blob.Write(clientChallenge)
	blob.Write([]byte{0, 0, 0, 0})
	blob.Write(targetInfo)
	blob.Write([]byte{0, 0, 0, 0})
	proof := hmacMD5(hash, append(append([]byte{}, serverChallenge...), blob.Bytes()...))
	return append(proof, blob.Bytes()...)
}

// avTimestamp is the server's MsvAvTimestamp, else now, as a FILETIME.
func avTimestamp(targetInfo []byte, now time.Time) []byte {
	for i := 0; i+4 <= len(targetInfo); {
		id := binary.LittleEndian.Uint16(targetInfo[i:])
		n := int(binary.LittleEndian.Uint16(targetInfo[i+2:]))
		if id == 0 || i+4+n > len(targetInfo) {
			break
		}
		if id == 7 && n == 8 {
			return targetInfo[i+4 : i+12]
		}
		i += 4 + n
	}
	ts := make([]byte, 8)
	binary.LittleEndian.PutUint64(ts, uint64(now.UnixNano()/100)+116444736000000000) //nolint:gosec // G115: a time after 1970
	return ts
}

func hmacMD5(key, data []byte) []byte {
	m := hmac.New(md5.New, key)
	m.Write(data)
	return m.Sum(nil)
}

func utf16le(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return b
}
