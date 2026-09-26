// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"context"
	"io"

	"golang.org/x/time/rate"
)

// readChunk bounds a single Read so the limiter is asked for a burst it can
// grant in one call instead of stalling on an oversized request.
const readChunk = 32 * 1024

// rateLimitedReader throttles reads to a byte rate using a token bucket;
// the burst equals the rate so a one-second read is never delayed further.
// It is used both for MaxSendSpeed (wrapping the request body) and
// MaxRecvSpeed (wrapping the response body).
type rateLimitedReader struct {
	r   io.Reader
	lim *rate.Limiter
	ctx context.Context
}

// newRateLimitedReader wraps r to cap its throughput at bytesPerSecond; a
// non-positive rate returns r unchanged.
func newRateLimitedReader(ctx context.Context, r io.Reader, bytesPerSecond int64) io.Reader {
	if bytesPerSecond <= 0 {
		return r
	}
	burst := int(bytesPerSecond)
	if burst <= 0 {
		burst = 1
	}
	return &rateLimitedReader{r: r, lim: rate.NewLimiter(rate.Limit(bytesPerSecond), burst), ctx: ctx}
}

func (l *rateLimitedReader) Read(p []byte) (int, error) {
	// A read never exceeds the burst, which the limiter could not grant.
	if limit := min(readChunk, l.lim.Burst()); len(p) > limit {
		p = p[:limit]
	}
	n, err := l.r.Read(p)
	if n > 0 {
		if werr := l.lim.WaitN(l.ctx, n); werr != nil {
			return n, werr
		}
	}
	return n, err
}

// Close lets rateLimitedReader stand in for an io.ReadCloser body.
func (l *rateLimitedReader) Close() error {
	if c, ok := l.r.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
