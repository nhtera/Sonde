// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package h1wire

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/nhtera/sonde/exchange"
)

// Limits of a response head.
const (
	// maxHeadBytes caps the status line and headers of one response (and
	// the trailers of a chunked body), as net/http's default does.
	maxHeadBytes = 1 << 20
	// maxInformational caps the 1xx responses read before the final one.
	maxInformational = 10
	// maxFields caps the header (or trailer) lines of one response.
	maxFields = 1000
	// maxChunkLine caps one chunk-size line (size and extensions).
	maxChunkLine = 4096
)

// ResponseError is a response sonde does not read: a malformed status
// line, framing it cannot trust, or a head over the size limit.
type ResponseError struct{ Msg string }

func (e *ResponseError) Error() string { return e.Msg }

func responseErrorf(format string, args ...any) error {
	return &ResponseError{Msg: fmt.Sprintf(format, args...)}
}

// Body framings.
const (
	bodyNone = iota
	bodyFixed
	bodyChunked
	bodyToClose // read until the server closes the connection
)

// responseHead is a parsed status line and headers, with the framing of
// the body that follows.
type responseHead struct {
	minor   int
	status  int
	reason  string
	headers []exchange.Header // wire order, names as received

	framing int
	length  int64 // bodyFixed
	// keepAlive tells whether the connection may carry another request
	// once the body is read.
	keepAlive bool
	// anomaly reports something read leniently (a header name that is not
	// a token, a folded line, a line without a colon): the connection is
	// not reused.
	anomaly bool
}

// statusText is the status as net/http writes it: "200 OK".
func (h *responseHead) statusText() string {
	if h.reason == "" {
		return strconv.Itoa(h.status)
	}
	return strconv.Itoa(h.status) + " " + h.reason
}

// readLine reads one line, without its LF and an optional CR before it,
// counting its bytes against budget.
func readLine(br *bufio.Reader, budget *int) (string, error) {
	var b []byte
	for {
		chunk, err := br.ReadSlice('\n')
		*budget -= len(chunk)
		if *budget < 0 {
			return "", responseErrorf("response head larger than %d bytes", maxHeadBytes)
		}
		b = append(b, chunk...)
		if err == nil {
			break
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			if errors.Is(err, io.EOF) && len(b) > 0 {
				err = io.ErrUnexpectedEOF
			}
			return "", err
		}
	}
	b = b[:len(b)-1]
	if n := len(b); n > 0 && b[n-1] == '\r' {
		b = b[:n-1]
	}
	return string(b), nil
}

// readHead reads one response head (informational or final) answering a
// request with method.
func readHead(br *bufio.Reader, method string) (*responseHead, error) {
	budget := maxHeadBytes
	line, err := readLine(br, &budget)
	if err != nil {
		return nil, err
	}
	h, err := parseStatusLine(line)
	if err != nil {
		return nil, err
	}
	if h.headers, h.anomaly, err = readFields(br, &budget); err != nil {
		return nil, err
	}
	return h, h.frame(method)
}

// parseStatusLine reads "HTTP/1.x SSS reason".
func parseStatusLine(line string) (*responseHead, error) {
	rest, ok := strings.CutPrefix(line, "HTTP/1.")
	if !ok || len(rest) < 5 || (rest[0] != '0' && rest[0] != '1') || rest[1] != ' ' {
		return nil, responseErrorf("malformed HTTP/1.x status line %q", truncate(line))
	}
	h := &responseHead{minor: int(rest[0] - '0')}
	code := rest[2:5]
	for i := 0; i < 3; i++ {
		if code[i] < '0' || code[i] > '9' {
			return nil, responseErrorf("malformed status code in %q", truncate(line))
		}
	}
	h.status, _ = strconv.Atoi(code)
	if h.status < 100 {
		return nil, responseErrorf("malformed status code in %q", truncate(line))
	}
	switch tail := rest[5:]; {
	case tail == "":
	case tail[0] == ' ':
		h.reason = tail[1:]
	default:
		return nil, responseErrorf("malformed status code in %q", truncate(line))
	}
	return h, nil
}

// readFields reads header (or trailer) lines up to the blank line. Names
// are kept as received; one that is not a token, a folded line or a line
// without a colon is an anomaly, read as best it can be.
func readFields(br *bufio.Reader, budget *int) (fields []exchange.Header, anomaly bool, err error) {
	for {
		line, err := readLine(br, budget)
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return nil, false, err
		}
		if line == "" {
			return fields, anomaly, nil
		}
		if len(fields) >= maxFields {
			return nil, false, responseErrorf("more than %d header lines", maxFields)
		}
		if line[0] == ' ' || line[0] == '\t' { // obsolete line folding
			if len(fields) == 0 {
				return nil, false, responseErrorf("folded header line without a header")
			}
			last := &fields[len(fields)-1]
			last.Value = strings.TrimRight(last.Value+" "+strings.Trim(line, " \t"), " \t")
			anomaly = true
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok || name == "" {
			anomaly = true
			continue
		}
		if !isToken(name) {
			anomaly = true
		}
		fields = append(fields, exchange.Header{Name: name, Value: strings.Trim(value, " \t")})
	}
}

// framingName returns "content-length" or "transfer-encoding" for a
// header that frames the body, and reports whether name only nearly is
// one (spaces or controls around it), which is refused: it could be read
// differently by another party.
func framingName(name string) (exact string, nearMiss bool) {
	lower := strings.ToLower(name)
	if lower == "content-length" || lower == "transfer-encoding" {
		return lower, false
	}
	trimmed := strings.TrimFunc(lower, func(r rune) bool { return r <= ' ' || r == 0x7f })
	return "", trimmed == "content-length" || trimmed == "transfer-encoding"
}

// frame sets the framing of the body and whether the connection can be
// reused: Transfer-Encoding wins over Content-Length (and closes the
// connection when both are present); without either, the body runs to
// the end of the connection.
func (h *responseHead) frame(method string) error {
	var lengths, codings []string
	closeConn, keepAliveToken := false, false
	for _, f := range h.headers {
		name, nearMiss := framingName(f.Name)
		if nearMiss {
			return responseErrorf("malformed framing header %q", f.Name)
		}
		switch name {
		case "content-length":
			lengths = append(lengths, f.Value)
		case "transfer-encoding":
			codings = append(codings, f.Value)
		}
		if strings.EqualFold(f.Name, "Connection") {
			for tok := range strings.SplitSeq(f.Value, ",") {
				switch strings.ToLower(strings.TrimSpace(tok)) {
				case "close":
					closeConn = true
				case "keep-alive":
					keepAliveToken = true
				}
			}
		}
	}
	h.keepAlive = !closeConn && (h.minor == 1 || keepAliveToken)

	tunnel := method == "CONNECT" && h.status >= 200 && h.status < 300
	if (h.status >= 100 && h.status < 200) || h.status == 204 || h.status == 304 || method == "HEAD" || tunnel {
		h.framing = bodyNone
		if h.status == 101 {
			h.keepAlive = false
		}
		return nil
	}
	if len(codings) > 0 {
		var all []string
		for _, c := range codings {
			for tok := range strings.SplitSeq(c, ",") {
				if tok = strings.TrimSpace(tok); tok != "" {
					all = append(all, strings.ToLower(tok))
				}
			}
		}
		if len(all) > 0 && all[len(all)-1] == "chunked" {
			h.framing = bodyChunked
		} else {
			h.framing, h.keepAlive = bodyToClose, false
		}
		if len(lengths) > 0 {
			h.keepAlive = false
		}
		return nil
	}
	if len(lengths) > 0 {
		n, err := contentLength(lengths)
		if err != nil {
			return err
		}
		h.framing, h.length = bodyFixed, n
		return nil
	}
	h.framing, h.keepAlive = bodyToClose, false
	return nil
}

// contentLength reads the Content-Length values: digits only, all equal.
func contentLength(values []string) (int64, error) {
	var n int64 = -1
	for _, v := range values {
		if v == "" || strings.TrimLeft(v, "0123456789") != "" || len(v) > 18 {
			return 0, responseErrorf("invalid Content-Length %q", truncate(v))
		}
		m, _ := strconv.ParseInt(v, 10, 64)
		if n >= 0 && m != n {
			return 0, responseErrorf("conflicting Content-Length values")
		}
		n = m
	}
	return n, nil
}

// truncate shortens s for an error message.
func truncate(s string) string {
	if len(s) > 80 {
		return s[:80] + "…"
	}
	return s
}

// fixedReader reads exactly n bytes; an earlier end is unexpected.
type fixedReader struct {
	r io.Reader
	n int64
}

func (f *fixedReader) Read(p []byte) (int, error) {
	if f.n <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > f.n {
		p = p[:f.n]
	}
	n, err := f.r.Read(p)
	f.n -= int64(n)
	if errors.Is(err, io.EOF) {
		if f.n > 0 {
			return n, io.ErrUnexpectedEOF
		}
		err = nil
	}
	if err == nil && f.n == 0 {
		err = io.EOF
	}
	return n, err
}

// chunkedReader decodes a chunked body and reads its trailers.
type chunkedReader struct {
	br       *bufio.Reader
	n        int64 // bytes left in the current chunk
	started  bool
	done     bool
	err      error
	trailers []exchange.Header
	budget   int
	anomaly  bool
}

func (c *chunkedReader) Read(p []byte) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	if c.done {
		return 0, io.EOF
	}
	if c.n == 0 {
		if err := c.nextChunk(); err != nil {
			c.err = err
			return 0, err
		}
		if c.done {
			return 0, io.EOF
		}
	}
	if int64(len(p)) > c.n {
		p = p[:c.n]
	}
	n, err := c.br.Read(p)
	c.n -= int64(n)
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		c.err = err
	}
	return n, err
}

// nextChunk ends the previous chunk and reads the size of the next; a
// zero size reads the trailers.
func (c *chunkedReader) nextChunk() error {
	if c.started {
		if err := c.crlf(); err != nil {
			return err
		}
	}
	c.started = true
	lineBudget := maxChunkLine // each size line on its own; c.budget is the trailers'
	line, err := readLine(c.br, &lineBudget)
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return err
	}
	size, _, _ := strings.Cut(line, ";")
	size = strings.TrimRight(size, " \t")
	if size == "" || len(size) > 15 || strings.TrimLeft(size, "0123456789abcdefABCDEF") != "" {
		return responseErrorf("invalid chunk size %q", truncate(line))
	}
	c.n, _ = strconv.ParseInt(size, 16, 64)
	if c.n == 0 {
		trailers, anomaly, err := readFields(c.br, &c.budget)
		if err != nil {
			return err
		}
		c.trailers, c.anomaly, c.done = trailers, anomaly, true
	}
	return nil
}

// crlf reads the line end after a chunk's data.
func (c *chunkedReader) crlf() error {
	b, err := c.br.ReadByte()
	if err == nil && b == '\r' {
		b, err = c.br.ReadByte()
	}
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return err
	}
	if b != '\n' {
		return responseErrorf("missing line end after a chunk")
	}
	return nil
}
