// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package grpcx

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// MaxMessageSize is the largest message received, as in gRPC
// implementations.
const MaxMessageSize = 4 << 20

// Frame prefixes an uncompressed message with its 5-byte header: a
// compression flag and a big-endian length.
func Frame(msg []byte) []byte {
	out := make([]byte, 5+len(msg))
	binary.BigEndian.PutUint32(out[1:5], uint32(len(msg))) //nolint:gosec // G115: request messages are far below 4 GiB
	copy(out[5:], msg)
	return out
}

// Parser splits a response body into messages, decompressing them.
type Parser struct {
	gzip bool
	buf  []byte
}

// NewParser returns a parser for the grpc-encoding of a response.
func NewParser(encoding string) (*Parser, error) {
	switch encoding {
	case "", "identity":
		return &Parser{}, nil
	case "gzip":
		return &Parser{gzip: true}, nil
	}
	return nil, fmt.Errorf("unsupported grpc-encoding %q", encoding)
}

// Write adds bytes read and returns the messages they complete.
func (p *Parser) Write(chunk []byte) ([][]byte, error) {
	p.buf = append(p.buf, chunk...)
	var msgs [][]byte
	for len(p.buf) >= 5 {
		n := binary.BigEndian.Uint32(p.buf[1:5])
		if n > MaxMessageSize {
			return msgs, fmt.Errorf("received a message of %d bytes, more than the limit of %d", n, MaxMessageSize)
		}
		if uint32(len(p.buf)-5) < n { //nolint:gosec // G115: len is not negative
			break
		}
		flag, data := p.buf[0], p.buf[5:5+n]
		msg, err := p.message(flag, data)
		if err != nil {
			return msgs, err
		}
		msgs = append(msgs, msg)
		p.buf = p.buf[5+n:]
	}
	return msgs, nil
}

// Close reports a message cut by the end of the body.
func (p *Parser) Close() error {
	if len(p.buf) > 0 {
		return errors.New("the response ended inside a message")
	}
	return nil
}

func (p *Parser) message(flag byte, data []byte) ([]byte, error) {
	switch {
	case flag == 0:
		return bytes.Clone(data), nil
	case flag != 1:
		return nil, fmt.Errorf("invalid message compression flag %d", flag)
	case !p.gzip:
		return nil, errors.New("received a compressed message without grpc-encoding")
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("could not uncompress a message: %w", err)
	}
	out, err := io.ReadAll(io.LimitReader(r, MaxMessageSize+1))
	if err != nil {
		return nil, fmt.Errorf("could not uncompress a message: %w", err)
	}
	if len(out) > MaxMessageSize {
		return nil, fmt.Errorf("received a message of more than %d bytes", MaxMessageSize)
	}
	return out, nil
}
