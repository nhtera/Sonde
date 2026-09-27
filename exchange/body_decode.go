// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/nhtera/sonde/internal/charset"
	"github.com/nhtera/sonde/internal/codec"
)

// DefaultMaxDecodedBody is the decoded (decompressed) body size limit used
// when Response.MaxDecodedBody is zero.
const DefaultMaxDecodedBody = 512 << 20

// BodyErrorKind classifies body decoding errors.
type BodyErrorKind int

// Body decoding error kinds.
const (
	// UnsupportedEncoding: a content coding sonde cannot decode.
	UnsupportedEncoding BodyErrorKind = iota
	// DecompressFailed: the body is not valid for its content coding.
	DecompressFailed
	// BodyTooLarge: the decoded body exceeds the size limit.
	BodyTooLarge
	// InvalidCharset: the Content-Type charset is unknown.
	InvalidCharset
	// InvalidDecoding: the body is not valid text in its charset.
	InvalidDecoding
)

// BodyError is an error decoding a response body. Name is the content
// coding or charset involved; Limit is the size limit for BodyTooLarge.
type BodyError struct {
	Kind  BodyErrorKind
	Name  string
	Limit int64
}

func (e *BodyError) Error() string { return e.Description() + ": " + e.Message() }

// Description is a short title for the error.
func (e *BodyError) Description() string {
	switch e.Kind {
	case InvalidCharset:
		return "Invalid charset"
	case InvalidDecoding:
		return "Invalid decoding"
	}
	return "Decompression error"
}

// Message explains the error.
func (e *BodyError) Message() string {
	switch e.Kind {
	case UnsupportedEncoding:
		return fmt.Sprintf("compression %s is not supported", e.Name)
	case DecompressFailed:
		return "could not uncompress response with " + e.Name
	case BodyTooLarge:
		return fmt.Sprintf("decoded body is larger than %d bytes", e.Limit)
	case InvalidCharset:
		return fmt.Sprintf("the charset '%s' is not valid", e.Name)
	}
	return fmt.Sprintf("could not decode response body with charset '%s'", e.Name)
}

// ContentEncodings returns the codings listed by the first Content-Encoding
// header, in header order.
func (r *Response) ContentEncodings() []string {
	v, ok := r.Headers.Get("Content-Encoding")
	if !ok {
		return nil
	}
	var codings []string
	for c := range strings.SplitSeq(v, ",") {
		codings = append(codings, strings.TrimSpace(c))
	}
	return codings
}

// DecodedBody returns the body with every content coding removed
// (br, gzip, deflate, zstd, identity), undoing the last applied coding first.
func (r *Response) DecodedBody() ([]byte, error) {
	limit := r.MaxDecodedBody
	if limit <= 0 {
		limit = DefaultMaxDecodedBody
	}
	codings := r.ContentEncodings()
	data := r.Body
	for i := len(codings) - 1; i >= 0; i-- {
		var err error
		if data, err = decode(codings[i], data, limit); err != nil {
			return nil, err
		}
	}
	return data, nil
}

func decode(coding string, data []byte, limit int64) ([]byte, error) {
	if coding == "identity" {
		return data, nil
	}
	rd, release, err := codec.NewReader(coding, bytes.NewReader(data))
	defer release()
	if errors.Is(err, codec.ErrUnsupported) {
		return nil, &BodyError{Kind: UnsupportedEncoding, Name: coding}
	}
	fail := &BodyError{Kind: DecompressFailed, Name: codec.Name(coding)}
	if err != nil {
		return nil, fail
	}
	out, err := io.ReadAll(io.LimitReader(rd, limit+1))
	if err != nil {
		return nil, fail
	}
	if int64(len(out)) > limit {
		return nil, &BodyError{Kind: BodyTooLarge, Limit: limit}
	}
	return out, nil
}

// Charset returns the charset parameter of the Content-Type header.
func (r *Response) Charset() (string, bool) {
	ct, ok := r.ContentType()
	if !ok {
		return "", false
	}
	for part := range strings.SplitSeq(strings.TrimSpace(ct), ";") {
		kv := strings.Split(strings.TrimSpace(part), "=")
		if len(kv) == 2 && strings.EqualFold(strings.TrimSpace(kv[0]), "charset") {
			return strings.TrimSpace(kv[1]), true
		}
	}
	return "", false
}

// Text returns the decoded body as a string, using the charset of the
// Content-Type header (UTF-8 by default). Malformed input is an error.
func (r *Response) Text() (string, error) {
	body, err := r.DecodedBody()
	if err != nil {
		return "", err
	}
	label, ok := r.Charset()
	if !ok {
		label = "utf-8"
	}
	enc, ok := charset.Lookup(label)
	if !ok {
		return "", &BodyError{Kind: InvalidCharset, Name: label}
	}
	s, ok := charset.Decode(enc, body)
	if !ok {
		return "", &BodyError{Kind: InvalidDecoding, Name: charset.Name(enc)}
	}
	return s, nil
}
