// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package exchange is the transport-neutral model of an HTTP exchange: the
// request that was sent and the response that was received, with the
// helpers evaluation needs (decoded body, text, cookies).
//
// Headers are kept in the order the transport delivered them; lookups by
// name are case-insensitive.
package exchange

import (
	"strings"
	"time"
)

// Header is a header field.
type Header struct {
	Name  string
	Value string
}

// Headers is a list of header fields in received order.
type Headers []Header

// Get returns the value of the first field named name (case-insensitive).
func (h Headers) Get(name string) (string, bool) {
	for _, f := range h {
		if strings.EqualFold(f.Name, name) {
			return f.Value, true
		}
	}
	return "", false
}

// Values returns the values of every field named name (case-insensitive).
func (h Headers) Values(name string) []string {
	var vs []string
	for _, f := range h {
		if strings.EqualFold(f.Name, name) {
			vs = append(vs, f.Value)
		}
	}
	return vs
}

// Request is a request as sent.
type Request struct {
	Method  string
	URL     string
	Headers Headers
	Body    []byte
}

// Response is a response as received. Body holds the bytes as transferred,
// before any content decoding.
type Response struct {
	// Version is "HTTP/1.0", "HTTP/1.1", "HTTP/2" or "HTTP/3".
	Version string
	Status  int
	// Reason is the reason phrase of the status line ("OK"); empty for
	// HTTP/2 and HTTP/3.
	Reason  string
	Headers Headers
	Body    []byte
	// URL is the URL this response was received from.
	URL string
	// IP is the address of the server.
	IP string
	// Duration is the transfer time of this response.
	Duration time.Duration
	Timings  Timings
	// Certificate is the server certificate of an HTTPS response.
	Certificate *CertInfo
	// MaxDecodedBody limits the size of the decoded body in bytes; zero
	// means DefaultMaxDecodedBody.
	MaxDecodedBody int64
	// Stream is set for a streamed entry (Server-Sent Events read with a
	// sonde-stream-* option, a WebSocket exchange or a server-streaming
	// gRPC call); nil otherwise.
	Stream *Stream
	// GRPC is the status of a gRPC call; nil for any other entry, and for
	// a gRPC stream stopped before the server sent its status.
	GRPC *GRPCStatus
}

// Timings are the phases of a transfer, measured from its start.
type Timings struct {
	Begin         time.Time
	End           time.Time
	NameLookup    time.Duration
	Connect       time.Duration
	AppConnect    time.Duration
	PreTransfer   time.Duration
	StartTransfer time.Duration
	Total         time.Duration
}

// CertInfo describes a server certificate. Empty strings and zero times
// mean the attribute is not available.
type CertInfo struct {
	Subject        string
	Issuer         string
	StartDate      time.Time
	ExpireDate     time.Time
	SerialNumber   string
	SubjectAltName string
	// Value is the certificate in PEM format.
	Value string
}

// ContentType returns the first Content-Type header value.
func (r *Response) ContentType() (string, bool) {
	return r.Headers.Get("Content-Type")
}

// IsHTML reports whether the content type is text/html.
func (r *Response) IsHTML() bool {
	ct, ok := r.ContentType()
	return ok && strings.HasPrefix(strings.ToLower(strings.TrimSpace(ct)), "text/html")
}
