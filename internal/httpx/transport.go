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

	"github.com/nhtera/sonde/internal/sandbox"
)

// builtTransport is a cached transport together with the pieces Execute
// needs alongside it.
type builtTransport struct {
	rt       http.RoundTripper
	insecure bool
}

// transportCacheKey identifies the transport options that determine how a
// connection is made and secured; requests that agree on all of these can
// reuse the same *http.Transport (and its connection pool).
func transportCacheKey(opts *Options) string {
	var b strings.Builder
	fmt.Fprintf(&b, "insecure=%v;cacert=%s;cert=%s;key=%s;pin=%s;",
		opts.Insecure, opts.CACert, opts.ClientCert, opts.ClientKey, opts.PinnedPublicKey)
	fmt.Fprintf(&b, "proxy=%s;noproxy=%s;unix=%s;connectto=%s;resolve=%s;ip=%d;http=%d;connto=%s",
		opts.Proxy, opts.NoProxy, opts.UnixSocket,
		strings.Join(opts.ConnectTo, ","), strings.Join(opts.Resolve, ","),
		opts.IPResolve, opts.HTTPVersion, opts.ConnectTimeout)
	return b.String()
}

// buildTransport constructs the RoundTripper for one distinct set of
// transport options; the server certificate must match tlsHost.
func buildTransport(opts *Options, cfg ClientConfig, tlsHost string) (*builtTransport, error) {
	dialOpts, err := newDialOptions(opts, cfg.Sandbox)
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
			return environmentProxy(r.URL, opts.NoProxy), nil
		},
	}

	switch opts.HTTPVersion {
	case HTTP11:
		t.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
		t.ForceAttemptHTTP2 = false
	case HTTP2:
		t.ForceAttemptHTTP2 = true
		// There is no h2c: an http:// request asking for HTTP/2 is sent
		// with HTTP/1.1, as a server refusing the upgrade would answer.
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
			dc, err := socks5DialContext(proxyURL, opts.NoProxy, dialOpts.dialContext())
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

	return &builtTransport{rt: t, insecure: opts.Insecure}, nil
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
