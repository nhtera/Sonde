// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package grpcx

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/nhtera/sonde/exchange"
)

// RequestHeaders are the headers every call sends: the content type,
// trailers support, the encodings accepted and, unless the file sets its
// own grpc-timeout, the deadline.
func RequestHeaders(timeout time.Duration, deadline bool) []exchange.Header {
	h := []exchange.Header{
		{Name: "content-type", Value: "application/grpc"},
		{Name: "te", Value: "trailers"},
		{Name: "grpc-accept-encoding", Value: "gzip"},
	}
	if deadline {
		h = append(h, exchange.Header{Name: "grpc-timeout", Value: Timeout(timeout)})
	}
	return h
}

// Reserved reports whether a request header is set by Sonde, so a file may
// not set it. grpc-timeout is the exception: it replaces the deadline sent.
func Reserved(name string) bool {
	name = strings.ToLower(name)
	return name == "content-type" || name == "te" || strings.HasPrefix(name, "grpc-") && name != "grpc-timeout"
}

// Timeout writes a grpc-timeout value: at most 8 digits and a unit, the
// largest unit that is exact, or else the smallest that fits, rounded up.
func Timeout(d time.Duration) string {
	const maxValue = 99999999
	units := []struct {
		unit string
		size time.Duration
	}{{"n", time.Nanosecond}, {"u", time.Microsecond}, {"m", time.Millisecond}, {"S", time.Second}, {"M", time.Minute}, {"H", time.Hour}}
	out := ""
	for _, u := range units {
		v := d / u.size
		if d%u.size != 0 {
			v++
		}
		if v <= maxValue && (out == "" || d%u.size == 0) {
			out = strconv.FormatInt(int64(v), 10) + u.unit
		}
	}
	if out == "" {
		return strconv.Itoa(maxValue) + "H"
	}
	return out
}

// ParseTimeout reads a grpc-timeout value; ok is false for an invalid one.
func ParseTimeout(v string) (d time.Duration, ok bool) {
	units := map[byte]time.Duration{'n': time.Nanosecond, 'u': time.Microsecond, 'm': time.Millisecond,
		'S': time.Second, 'M': time.Minute, 'H': time.Hour}
	if len(v) < 2 || len(v) > 9 {
		return 0, false
	}
	digits := v[:len(v)-1]
	size, ok := units[v[len(v)-1]]
	if !ok || strings.Trim(digits, "0123456789") != "" {
		return 0, false
	}
	n, _ := strconv.ParseInt(digits, 10, 64) // at most 8 digits
	if time.Duration(n) > math.MaxInt64/size {
		return math.MaxInt64, true
	}
	return time.Duration(n) * size, true
}
