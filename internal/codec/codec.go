// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package codec opens readers that undo HTTP content codings (br, gzip,
// deflate, zstd, identity), for whole bodies and for streams alike.
package codec

import (
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// zstdMaxWindow bounds the memory a zstd frame may ask for (the limit
// RFC 8878 recommends for decoders).
const zstdMaxWindow = 8 << 20

// ErrUnsupported is returned for a content coding no reader exists for.
var ErrUnsupported = errors.New("unsupported content coding")

// NewReader returns a reader undoing coding on r, and a function releasing
// its resources. A reader that fails to start (a bad header) returns the
// decoder's error.
func NewReader(coding string, r io.Reader) (io.Reader, func(), error) {
	noop := func() {}
	switch coding {
	case "identity":
		return r, noop, nil
	case "br":
		return brotli.NewReader(r), noop, nil
	case "gzip":
		rd, err := gzip.NewReader(r)
		return rd, noop, err
	case "deflate":
		rd, err := zlib.NewReader(r)
		return rd, noop, err
	case "zstd":
		d, err := zstd.NewReader(r, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxWindow(zstdMaxWindow))
		if err != nil {
			return nil, noop, err
		}
		return d, d.Close, nil
	}
	return nil, noop, ErrUnsupported
}

// Chain undoes codings, listed in header order (the last applied first).
func Chain(codings []string, r io.Reader) (io.Reader, func(), error) {
	var releases []func()
	release := func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}
	for i := len(codings) - 1; i >= 0; i-- {
		rd, rel, err := NewReader(codings[i], r)
		releases = append(releases, rel)
		if err != nil {
			release()
			return nil, func() {}, err
		}
		r = rd
	}
	return r, release, nil
}

// Name is how messages name a coding: its library, e.g. zlib for deflate.
func Name(coding string) string {
	switch coding {
	case "deflate":
		return "zlib"
	case "br":
		return "brotli"
	}
	return coding
}
