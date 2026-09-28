// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package httpx executes requests: it builds the HTTP request from a
// RequestSpec, applies the transport options, follows redirects manually
// and keeps the cookie store of a run unit.
package httpx

import (
	"io"
	"time"

	"github.com/nhtera/sonde/exchange"
	"github.com/nhtera/sonde/internal/sandbox"
)

// Param is a query string or form parameter.
type Param struct {
	Name  string
	Value string
}

// FileParam is a multipart file part.
type FileParam struct {
	Name        string
	Filename    string
	Data        []byte
	ContentType string
}

// MultipartParam is a multipart part: exactly one of Param and File is set.
type MultipartParam struct {
	Param *Param
	File  *FileParam
}

// RequestCookie is a cookie from a [Cookies] section.
type RequestCookie struct {
	Name  string
	Value string
}

// BodyKind tells how a body was written.
type BodyKind int

// Body kinds.
const (
	BodyBinary BodyKind = iota // base64, hex or no body
	BodyText                   // strings, JSON, XML, multiline
	BodyFile                   // file, ...;
)

// Body is a request body.
type Body struct {
	Kind     BodyKind
	Data     []byte
	Filename string // BodyFile only, as written
}

// RequestSpec is a request as described by an entry, after rendering its
// templates.
type RequestSpec struct {
	Method    string
	URL       string
	Headers   []exchange.Header
	Query     []Param
	Form      []Param
	Multipart []MultipartParam
	Cookies   []RequestCookie
	Body      Body
	// ImplicitContentType is the Content-Type sent when Headers has none:
	// form, multipart, JSON or XML bodies.
	ImplicitContentType string
}

// HTTPVersion is the requested protocol version.
type HTTPVersion int

// Requested versions. HTTP10 is not supported.
const (
	HTTPDefault HTTPVersion = iota
	HTTP10
	HTTP11
	HTTP2
	HTTP3
)

// IPResolve restricts name resolution.
type IPResolve int

// IP families.
const (
	IPAny IPResolve = iota
	IPv4
	IPv6
)

// Options are the transport options of one entry: command line options
// overridden by the entry's [Options] section. Paths come from the request
// file and are read through ClientConfig.Sandbox, except where noted.
type Options struct {
	AWSSigV4       string // unsupported: a non-empty value is an error
	CACert         string
	ClientCert     string
	ClientKey      string
	Compressed     bool
	ConnectTimeout time.Duration // zero: 300s
	ConnectTo      []string      // HOST1:PORT1:HOST2:PORT2
	Digest         bool          // unsupported
	FollowLocation bool
	// LocationTrusted forwards credentials to every redirect host.
	LocationTrusted bool
	Headers         []exchange.Header // -H/--header, added to every request
	HTTPVersion     HTTPVersion
	Insecure        bool
	IPResolve       IPResolve
	MaxFilesize     int64 // bytes, 0: no limit
	MaxRecvSpeed    int64 // bytes per second, 0: no limit
	MaxSendSpeed    int64 // bytes per second, 0: no limit
	MaxRedirects    int   // -1: unlimited; default 50
	Negotiate       bool  // unsupported
	Netrc           bool
	NetrcFile       string
	NetrcOptional   bool
	// NetrcAllowReroute sends netrc credentials even when resolve,
	// connect-to or a proxy reroutes the host.
	NetrcAllowReroute bool
	NoProxy           string
	NTLM              bool // unsupported
	PathAsIs          bool
	PinnedPublicKey   string
	Proxy             string
	Resolve           []string      // HOST:PORT:ADDR[,ADDR]...
	Timeout           time.Duration // max-time; zero: 300s
	UnixSocket        string
	User              string // user:password
	UserAgent         string // empty: sonde/<version>
	// Verbose enables the debug callback for connection details.
	Verbose bool
	// LocalFiles are the file options given on the command line
	// (cacert, cert, key, pinnedpubkey, unix-socket): they are read
	// relative to the working directory instead of through the sandbox.
	LocalFiles map[string]bool
	// ReadStream, when set, reads the body of the final response instead
	// of reading it whole (a streamed entry). header is the response's;
	// body is decoded (content codings removed); the response keeps the
	// bytes as received. MaxFilesize does not apply. stop cancels the
	// request, so a pending read returns an error. When it fails, Execute
	// returns the call with the error.
	ReadStream func(header exchange.Headers, body io.Reader, stop func()) error
	// GRPC marks a gRPC call: it uses HTTP/2 only (cleartext HTTP/2 with
	// prior knowledge for http://), never follows redirects, and its
	// response headers include the trailers.
	GRPC bool
}

// Call is one HTTP exchange of an entry; redirects produce several.
type Call struct {
	Request  exchange.Request
	Response *exchange.Response
	Timings  exchange.Timings
}

// ClientConfig configures a Client for one run unit.
type ClientConfig struct {
	// Sandbox confines the paths of Options and Body files. Required.
	Sandbox *sandbox.Root
	// CookieFile is a Netscape cookie file read at start (--cookie; a
	// command line path, not sandboxed). Empty: none.
	CookieFile string
	// NoCookieStore disables the cookie store.
	NoCookieStore bool
	// Version is the sonde version for the default User-Agent.
	Version string
	// UserAgent replaces the default `sonde/<version>` User-Agent.
	UserAgent string
	// Debug receives connection details (DNS, connect, TLS) as lines
	// without prefix when an entry is verbose. Nil: discarded.
	Debug func(line string)
	// Warn receives warnings (e.g. `insecure` once per client). Nil: discarded.
	Warn func(msg string)
}

// Cookie is a stored cookie, with the fields of the Netscape format.
type Cookie struct {
	Domain           string
	IncludeSubdomain bool
	Path             string
	HTTPS            bool
	Expires          int64 // Unix seconds, 0: session cookie
	Name             string
	Value            string
	HTTPOnly         bool
}

// Client executes the requests of one run unit and keeps its cookies. It is
// not safe for concurrent use.
type Client struct {
	cfg ClientConfig

	// transports caches one *http.Transport per distinct set of transport
	// options (TLS, proxy, dial), so requests that agree on those reuse
	// connections.
	transports map[string]*builtTransport
	jar        *cookieJar
	netrc      map[string]*netrcFile // by path, loaded when first needed
	warned     map[string]bool
}

// NewClient, Execute, Cookies, AddCookie, ClearCookies and Close are
// implemented in client.go and execute.go.

// ErrorKind classifies transport errors.
type ErrorKind int

// Transport error kinds; the message of each follows curl's wording where
// the command line tools print it.
const (
	ErrConnect          ErrorKind = iota // "(7) Failed to connect to …"
	ErrResolve                           // "(6) Could not resolve host: …"
	ErrTimeout                           // "(28) Operation timed out after …"
	ErrTLS                               // "(35)/(60) SSL …"
	ErrTooManyRedirects                  // "(47) Maximum (N) redirects followed"
	ErrMaxFilesize                       // "(63) Maximum file size exceeded"
	ErrInvalidURL                        // relative/malformed redirect or URL
	ErrUnsupported                       // unsupported option (http1.0, ntlm, …)
	ErrFileAccess                        // sandbox denial or unreadable file
	ErrOther
)

// Error is a transport error.
type Error struct {
	Kind ErrorKind
	// Description is the title (e.g. "HTTP connection", "Unsupported
	// option"); Msg the explanation.
	Description string
	Msg         string
	Err         error
}

func (e *Error) Error() string { return e.Description + ": " + e.Msg }

func (e *Error) Unwrap() error { return e.Err }
