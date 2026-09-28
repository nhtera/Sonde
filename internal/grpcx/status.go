// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package grpcx implements gRPC calls on top of an HTTP/2 client: message
// framing, status codes, message descriptors (.proto files, descriptor
// sets, server reflection) and the JSON mapping of messages
// (docs/decisions/0005-grpc.md).
package grpcx

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nhtera/sonde/exchange"
)

// Status codes used here by name.
const (
	CodeOK               = 0
	CodeCancelled        = 1
	CodeUnknown          = 2
	CodeDeadlineExceeded = 4
	CodeInternal         = 13
	CodeUnimplemented    = 12
	CodeUnavailable      = 14
)

var codeNames = [...]string{
	"OK", "CANCELLED", "UNKNOWN", "INVALID_ARGUMENT", "DEADLINE_EXCEEDED",
	"NOT_FOUND", "ALREADY_EXISTS", "PERMISSION_DENIED", "RESOURCE_EXHAUSTED",
	"FAILED_PRECONDITION", "ABORTED", "OUT_OF_RANGE", "UNIMPLEMENTED",
	"INTERNAL", "UNAVAILABLE", "DATA_LOSS", "UNAUTHENTICATED",
}

// CodeName is the name of a status code; "UNKNOWN" for a code gRPC does
// not define.
func CodeName(code int) string {
	if code >= 0 && code < len(codeNames) {
		return codeNames[code]
	}
	return "UNKNOWN"
}

// NewStatus builds a status.
func NewStatus(code int, message string) *exchange.GRPCStatus {
	return &exchange.GRPCStatus{Code: code, Status: CodeName(code), Message: message}
}

// StatusError is a call that ended with a status other than OK.
type StatusError struct {
	Status *exchange.GRPCStatus
}

func (e *StatusError) Error() string {
	if e.Status.Message == "" {
		return "gRPC status " + e.Status.Status
	}
	return "gRPC status " + e.Status.Status + ": " + e.Status.Message
}

// httpCodes maps the HTTP status of a response without grpc-status to a
// gRPC code, as the gRPC specification does (http-grpc-status-mapping).
var httpCodes = map[int]int{
	400: CodeInternal, 401: 16, 403: 7, 404: CodeUnimplemented,
	429: CodeUnavailable, 502: CodeUnavailable, 503: CodeUnavailable, 504: CodeUnavailable,
}

// IsGRPC reports whether a response is a gRPC reply: it has a gRPC content
// type, or a status of its own (a reply without messages).
func IsGRPC(h exchange.Headers) bool {
	if _, ok := h.Get("grpc-status"); ok {
		return true
	}
	ct, _ := h.Get("content-type")
	ct = strings.ToLower(strings.TrimSpace(ct))
	return ct == "application/grpc" || strings.HasPrefix(ct, "application/grpc+") || strings.HasPrefix(ct, "application/grpc;")
}

// Status is the status of a call that ended: from its grpc-status and
// grpc-message headers or trailers, else from its HTTP status or content
// type, as the gRPC specification maps them.
func Status(httpStatus int, h exchange.Headers) *exchange.GRPCStatus {
	if v, ok := h.Get("grpc-status"); ok {
		code, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return NewStatus(CodeUnknown, "invalid grpc-status "+strconv.Quote(v))
		}
		msg, _ := h.Get("grpc-message")
		return NewStatus(code, decodeMessage(msg))
	}
	if httpStatus != 200 {
		code, ok := httpCodes[httpStatus]
		if !ok {
			code = CodeUnknown
		}
		return NewStatus(code, fmt.Sprintf("HTTP status %d without grpc-status", httpStatus))
	}
	if !IsGRPC(h) {
		ct, _ := h.Get("content-type")
		return NewStatus(CodeUnknown, "the response is not gRPC (content type "+strconv.Quote(ct)+")")
	}
	return NewStatus(CodeInternal, "the server ended the call without grpc-status")
}

// resetCodes maps the error code of an HTTP/2 stream reset to a gRPC code
// (the gRPC specification's PROTOCOL-HTTP2 errors section).
var resetCodes = map[string]int{
	"REFUSED_STREAM": CodeUnavailable, "CANCEL": CodeCancelled,
	"ENHANCE_YOUR_CALM": 8, "INADEQUATE_SECURITY": 7,
}

// ResetStatus is the status of a call whose stream the server reset, when
// err is such a reset ("stream error: stream ID 1; CANCEL; …"). The HTTP/2
// client does not export its error type, so its text is matched.
func ResetStatus(err error) (*exchange.GRPCStatus, bool) {
	if err == nil {
		return nil, false
	}
	text := err.Error()
	i := strings.Index(text, "stream error: ")
	if i < 0 {
		return nil, false
	}
	parts := strings.Split(text[i:], "; ")
	if len(parts) < 2 {
		return nil, false
	}
	name := strings.TrimSpace(parts[1])
	code, ok := resetCodes[name]
	if !ok {
		code = CodeInternal
	}
	return NewStatus(code, "the HTTP/2 stream was reset ("+name+")"), true
}

// decodeMessage percent-decodes a grpc-message value; an invalid escape is
// kept as written, as the specification asks.
func decodeMessage(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(v))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
