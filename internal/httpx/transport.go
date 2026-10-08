// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nhtera/sonde/internal/httpx/h1wire"
	"github.com/nhtera/sonde/internal/sandbox"
)

// builtTransport is a cached transport together with the pieces Execute
// needs alongside it.
type builtTransport struct {
	rt       http.RoundTripper
	insecure bool
	// proxy returns the HTTP proxy a request goes through, if any.
	proxy func(*http.Request) (*url.URL, error)
	// std is net/http's transport (rt itself, or behind the dispatcher):
	// the WebSocket handshake uses it.
	std *http.Transport
}

// closeIdle closes the idle connections of the transport.
func (b *builtTransport) closeIdle() {
	if c, ok := b.rt.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

// transportCacheKey identifies the transport options that determine how a
// connection is made and secured; requests that agree on all of these can
// reuse the same *http.Transport (and its connection pool).
func transportCacheKey(opts *Options) string {
	var b strings.Builder
	fmt.Fprintf(&b, "insecure=%v;cacert=%s;cert=%s;key=%s;pin=%s;",
		opts.Insecure, opts.CACert, opts.ClientCert, opts.ClientKey, opts.PinnedPublicKey)
	fmt.Fprintf(&b, "proxy=%s;noproxy=%s;unix=%s;connectto=%s;resolve=%s;ip=%d;http=%d;connto=%s;grpc=%v",
		opts.Proxy, opts.NoProxy, opts.UnixSocket,
		strings.Join(opts.ConnectTo, ","), strings.Join(opts.Resolve, ","),
		opts.IPResolve, opts.HTTPVersion, opts.ConnectTimeout, opts.GRPC)
	// The CONNECT request of a pooled tunnel carries the proxy headers.
	for _, h := range opts.ProxyHeaders {
		fmt.Fprintf(&b, ";proxyheader=%q:%q", h.Name, h.Value)
	}
	return b.String()
}

// buildTransport constructs the RoundTripper for one distinct set of
// transport options; the server certificate must match tlsHost.
func buildTransport(opts *Options, cfg ClientConfig, tlsHost string) (*builtTransport, error) {
	dialOpts, err := newDialOptions(opts, cfg.Sandbox, cfg.Hosts)
	if err != nil {
		return nil, err
	}

	tlsConfig, err := buildTLSConfig(opts, cfg.Sandbox, cfg.Warn, tlsHost)
	if err != nil {
		return nil, err
	}

	t := &http.Transport{
		DialContext:        dialOpts.dialContext(),
		TLSClientConfig:    tlsConfig,
		DisableCompression: true,
		Proxy: func(r *http.Request) (*url.URL, error) {
			return environmentProxyErr(r.URL, opts.NoProxy)
		},
	}

	switch opts.HTTPVersion {
	case HTTP10, HTTP11:
		t.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
		t.ForceAttemptHTTP2 = false
	case HTTP2:
		t.ForceAttemptHTTP2 = true
		// There is no h2c upgrade: an http:// request asking for HTTP/2 is
		// sent with HTTP/1.1, as a server refusing the upgrade would answer.
	case HTTP2PriorKnowledge:
		// Cleartext HTTP/2 without an upgrade (Execute uses HTTP2 for
		// https:// URLs).
		t.Protocols = new(http.Protocols)
		t.Protocols.SetHTTP2(true)
		t.Protocols.SetUnencryptedHTTP2(true)
	default:
		t.ForceAttemptHTTP2 = true
	}

	if opts.Proxy != "" {
		proxyURL, err := parseProxyURL(opts.Proxy)
		if err != nil {
			return nil, err
		}
		switch proxyURL.Scheme {
		case "socks5", "socks5h":
			dc, err := socks5DialContext(proxyURL, opts.NoProxy, dialOpts.dialContext(), cfg.Hosts)
			if err != nil {
				return nil, err
			}
			t.DialContext = dc
			t.Proxy = nil // an explicit proxy replaces any from the environment
		case "http", "https":
			t.Proxy = proxyFunc(proxyURL, opts.NoProxy)
		default:
			return nil, invalidURLError(opts.Proxy, "unsupported proxy scheme "+proxyURL.Scheme)
		}
	}

	if len(opts.ProxyHeaders) > 0 {
		t.ProxyConnectHeader = http.Header{}
		for _, h := range opts.ProxyHeaders {
			t.ProxyConnectHeader.Add(h.Name, h.Value)
		}
	}
	built := &builtTransport{rt: t, insecure: opts.Insecure, proxy: t.Proxy, std: t}
	switch {
	case opts.GRPC:
		grpcTransport(t)
	case opts.HTTPVersion != HTTP2PriorKnowledge && !legacyWire():
		d := newDispatcher(t, opts, tlsConfig)
		if opts.HTTPVersion == HTTP3 {
			d.h3, d.h3Failed, d.connectTimeout = newH3Transport(dialOpts, tlsConfig), map[string]time.Time{}, dialOpts.connectTimeout
		}
		built.rt = d
	case opts.HTTPVersion == HTTP3:
		// Legacy wire: net/http for TCP, still QUIC first.
		d := &dispatcher{std: t, proxy: t.Proxy, legacy: true, h3: newH3Transport(dialOpts, tlsConfig),
			h3Failed: map[string]time.Time{}, connectTimeout: dialOpts.connectTimeout}
		built.rt = d
	}
	return built, nil
}

// newDispatcher puts h1wire in front of t for HTTP/1.x.
func newDispatcher(t *http.Transport, opts *Options, tlsConfig *tls.Config) *dispatcher {
	forced := opts.HTTPVersion == HTTP10 || opts.HTTPVersion == HTTP11
	d := &dispatcher{
		std:    t,
		forced: forced,
		proxy:  t.Proxy,
		h1: &h1wire.Transport{
			Dial:         t.DialContext,
			TLSConfig:    tlsConfig,
			Proxy:        t.Proxy,
			ProxyHeaders: opts.ProxyHeaders,
			OfferH2:      !forced,
		},
		h2Addrs: map[string]bool{},
		handoff: map[string][]*tls.Conn{},
	}
	if !forced {
		t.DialTLSContext = d.dialTLS(t.DialContext, tlsConfig)
	}
	return d
}

// grpcTransport restricts t to HTTP/2, with prior knowledge over
// cleartext: gRPC servers speak nothing else. Cleartext HTTP/2 cannot go
// through an HTTP proxy, which would receive the HTTP/2 preface itself.
func grpcTransport(t *http.Transport) {
	t.Protocols = new(http.Protocols)
	t.Protocols.SetHTTP2(true)
	t.Protocols.SetUnencryptedHTTP2(true)
	if proxy := t.Proxy; proxy != nil {
		t.Proxy = func(r *http.Request) (*url.URL, error) {
			u, err := proxy(r)
			if err == nil && u != nil && r.URL.Scheme == "http" {
				return nil, newError(ErrUnsupported, "Unsupported proxy",
					"a gRPC call to an http:// URL (cleartext HTTP/2) cannot go through the HTTP proxy "+u.Redacted(), nil)
			}
			return u, err
		}
	}
}

// buildTLSConfig assembles the tls.Config for --insecure, --cacert,
// --cert/--key and --pinned-public-key; the server certificate must match
// host.
func buildTLSConfig(opts *Options, box *sandbox.Root, warn func(string), host string) (*tls.Config, error) {
	// Verification is done in VerifyConnection (curlVerify) so that
	// certificates naming their host only in the subject Common Name are
	// accepted, as curl does.
	cfg := &tls.Config{InsecureSkipVerify: true} //nolint:gosec // G402: verified in VerifyConnection unless --insecure
	insecure := opts.Insecure
	if insecure && warn != nil {
		warn("insecure: server certificate verification disabled")
	}

	if opts.CACert != "" {
		data, err := readOptionFile(box, opts, opts.CACert)
		if err != nil {
			return nil, fileAccessError(opts.CACert, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(data) {
			return nil, sslSetupError(77, "Problem with the SSL CA cert (no certificate found in "+opts.CACert+")", nil)
		}
		cfg.RootCAs = pool
	}

	if opts.ClientCert != "" {
		certPath, password := splitCertPassword(opts.ClientCert)
		if opts.LocalFiles[opts.ClientCert] {
			opts = withLocal(opts, certPath)
		}
		certPEM, err := readOptionFile(box, opts, certPath)
		if err != nil {
			return nil, fileAccessError(certPath, err)
		}
		keyPath := opts.ClientKey
		var keyPEM []byte
		if keyPath != "" {
			keyPEM, err = readOptionFile(box, opts, keyPath)
			if err != nil {
				return nil, fileAccessError(keyPath, err)
			}
		} else {
			keyPEM = certPEM // key bundled in the same PEM file
		}
		if keyPEM, err = decryptKeyPEM(keyPEM, password); err != nil {
			return nil, sslSetupError(58, "Problem with the local SSL certificate: "+err.Error(), err)
		}
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, sslSetupError(58, "Problem with the local SSL certificate: "+err.Error(), err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}

	var checker *pinnedKeyChecker
	if opts.PinnedPublicKey != "" {
		checker = parsePinnedPublicKey(opts.PinnedPublicKey, func(name string) ([]byte, error) { return readOptionFile(box, opts, name) })
	}
	roots := cfg.RootCAs
	cfg.VerifyConnection = func(state tls.ConnectionState) error {
		if !insecure {
			if err := curlVerify(state, roots, host); err != nil {
				return err
			}
		}
		if checker != nil {
			return checker.verify(state)
		}
		return nil
	}
	return cfg, nil
}

// curlVerify verifies the server certificate chain against roots (nil:
// system roots) and host, the host of the URL (the connection state has
// no name for an IP address). A certificate without subject alternative
// names is matched on its Common Name, unless a certificate of its chain
// constrains names.
func curlVerify(state tls.ConnectionState, roots *x509.CertPool, host string) error {
	if len(state.PeerCertificates) == 0 {
		return errors.New("tls: no server certificate")
	}
	leaf := state.PeerCertificates[0]
	inter := x509.NewCertPool()
	for _, c := range state.PeerCertificates[1:] {
		inter.AddCert(c)
	}
	chains, err := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: inter})
	if err != nil {
		return &tls.CertificateVerificationError{UnverifiedCertificates: state.PeerCertificates, Err: err}
	}
	if host == "" {
		return &tls.CertificateVerificationError{UnverifiedCertificates: state.PeerCertificates,
			Err: errors.New("no host name to verify the certificate against")}
	}
	if len(leaf.DNSNames) > 0 || len(leaf.IPAddresses) > 0 || constrainsNames(chains) {
		if err := leaf.VerifyHostname(host); err != nil {
			return &tls.CertificateVerificationError{UnverifiedCertificates: state.PeerCertificates, Err: err}
		}
		return nil
	}
	if !strings.EqualFold(strings.TrimSuffix(leaf.Subject.CommonName, "."), strings.TrimSuffix(host, ".")) {
		return &tls.CertificateVerificationError{UnverifiedCertificates: state.PeerCertificates,
			Err: x509.HostnameError{Certificate: leaf, Host: host}}
	}
	return nil
}

// constrainsNames reports whether a certificate of a verified chain has
// name constraints, which do not apply to a Common Name.
func constrainsNames(chains [][]*x509.Certificate) bool {
	for _, chain := range chains {
		for _, c := range chain {
			if len(c.PermittedDNSDomains) > 0 || len(c.ExcludedDNSDomains) > 0 ||
				len(c.PermittedIPRanges) > 0 || len(c.ExcludedIPRanges) > 0 {
				return true
			}
		}
	}
	return false
}

// splitCertPassword splits `file:password` at the first colon not escaped
// with a backslash. On Windows, the colon of a leading drive letter
// (`C:\` or `C:/`) belongs to the file.
func splitCertPassword(spec string) (file, password string) {
	var b strings.Builder
	start := 0
	if runtime.GOOS == "windows" && hasDriveLetter(spec) {
		b.WriteString(spec[:2])
		start = 2
	}
	for i := start; i < len(spec); i++ {
		switch {
		case spec[i] == '\\' && i+1 < len(spec) && spec[i+1] == ':':
			b.WriteByte(':')
			i++
		case spec[i] == ':':
			return b.String(), spec[i+1:]
		default:
			b.WriteByte(spec[i])
		}
	}
	return b.String(), ""
}

// hasDriveLetter reports whether s starts with a drive letter and a colon
// followed by a separator.
func hasDriveLetter(s string) bool {
	if len(s) < 3 || s[1] != ':' || (s[2] != '\\' && s[2] != '/') {
		return false
	}
	c := s[0] | 0x20
	return c >= 'a' && c <= 'z'
}

// withLocal returns a copy of opts where name is also a command line path.
func withLocal(opts *Options, name string) *Options {
	o := *opts
	o.LocalFiles = map[string]bool{name: true}
	for k, v := range opts.LocalFiles {
		o.LocalFiles[k] = v
	}
	return &o
}

// isCertVerifyError reports whether err is a certificate verification
// failure (curl code 60) rather than a lower-level handshake error
// (curl code 35).
func isCertVerifyError(err error) bool {
	var unknownAuth x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	var invalid x509.CertificateInvalidError
	var verifyErr *tls.CertificateVerificationError
	if errors.As(err, &unknownAuth) || errors.As(err, &hostnameErr) || errors.As(err, &invalid) || errors.As(err, &verifyErr) {
		return true
	}
	return errors.Is(err, errPinMismatch)
}

// readOptionFile reads a file named by an option: command line paths
// directly, request-file paths through the sandbox. Either is relative to
// the working directory, as curl reads them.
func readOptionFile(box *sandbox.Root, opts *Options, name string) ([]byte, error) {
	if opts.LocalFiles[name] {
		return os.ReadFile(name) //nolint:gosec // G304: a command line path
	}
	if !filepath.IsAbs(name) {
		abs, err := filepath.Abs(name)
		if err != nil {
			return nil, err
		}
		name = abs
	}
	return box.ReadFile(name)
}
