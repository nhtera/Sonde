// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"errors"
	"fmt"
)

// newError builds an *Error, wrapping err when it is not nil.
func newError(kind ErrorKind, description, msg string, err error) *Error {
	return &Error{Kind: kind, Description: description, Msg: msg, Err: err}
}

// connectError reports a failed connection attempt, curl code (7).
func connectError(host string, port int, err error) *Error {
	msg := fmt.Sprintf("(7) Failed to connect to %s port %d", host, port)
	if err != nil {
		msg += ": " + err.Error()
	}
	return newError(ErrConnect, "HTTP connection", msg, err)
}

// resolveError reports a DNS resolution failure, curl code (6).
func resolveError(host string, err error) *Error {
	return newError(ErrResolve, "HTTP connection", "(6) Could not resolve host: "+host, err)
}

// timeoutError reports a transfer timeout, curl code (28).
func timeoutError(after string, err error) *Error {
	msg := fmt.Sprintf("(28) Operation timed out after %s", after)
	return newError(ErrTimeout, "HTTP connection", msg, err)
}

// tlsError reports a TLS handshake or verification failure, curl code (35)
// (handshake) or (60) (peer certificate could not be authenticated).
func tlsError(err error) *Error {
	if errors.Is(err, errPinMismatch) {
		return newError(ErrTLS, "HTTP connection", "(90) SSL: public key does not match pinned public key", err)
	}
	code := 35
	if isCertVerifyError(err) {
		code = 60
	}
	msg := fmt.Sprintf("(%d) SSL error: %s", code, err)
	return newError(ErrTLS, "HTTP connection", msg, err)
}

// tooManyRedirectsError reports a redirect loop past the configured limit.
func tooManyRedirectsError() *Error {
	return newError(ErrTooManyRedirects, "HTTP connection", "too many redirect", nil)
}

// maxFilesizeError reports a response body over --max-filesize, curl code (63).
func maxFilesizeError() *Error {
	return newError(ErrMaxFilesize, "HTTP connection", "(63) Maximum file size exceeded", nil)
}

// invalidURLError reports a malformed request or redirect URL.
func invalidURLError(rawURL, reason string) *Error {
	msg := fmt.Sprintf("invalid URL <%s> (%s)", rawURL, reason)
	return newError(ErrInvalidURL, "Invalid URL", msg, nil)
}

// unsupportedError reports an option sonde does not implement yet.
// sslSetupError reports unusable local TLS material, with curl's code.
func sslSetupError(code int, msg string, err error) *Error {
	return newError(ErrTLS, "HTTP connection", fmt.Sprintf("(%d) %s", code, msg), err)
}

func unsupportedError(name string) *Error {
	msg := fmt.Sprintf("option %q is not supported by sonde yet", name)
	return newError(ErrUnsupported, "Unsupported option", msg, nil)
}

// fileAccessError reports a sandbox denial or an unreadable file.
func fileAccessError(path string, err error) *Error {
	return newError(ErrFileAccess, "File access", "could not read "+path+": "+err.Error(), err)
}

// otherError wraps a transport failure that does not fit another kind.
func otherError(msg string, err error) *Error {
	return newError(ErrOther, "HTTP connection", msg, err)
}
