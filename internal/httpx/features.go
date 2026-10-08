// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

// featureProbes pairs each feature name a --version "Features:" line can
// list (the names curl-based tools print, which scripts grep for) with
// options that need it.
var featureProbes = []struct {
	name string
	opts Options
}{
	{"HTTP2", Options{HTTPVersion: HTTP2}},
	{"HTTP3", Options{HTTPVersion: HTTP3}},
	{"IPv6", Options{IPResolve: IPv6}},
	{"NTLM", Options{NTLM: true}},
	{"SPNEGO", Options{Negotiate: true}},
	{"UnixSockets", Options{UnixSocket: "socket"}},
}

// Features lists the optional transport features this build implements,
// derived from the same check that rejects unsupported options, so the
// list can never claim a feature a request would refuse.
func Features() []string {
	var out []string
	for _, p := range featureProbes {
		opts := p.opts
		if checkSupported(&opts) == nil {
			out = append(out, p.name)
		}
	}
	return out
}
