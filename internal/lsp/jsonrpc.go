// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// maxMessage bounds one incoming JSON-RPC message; request files are capped
// at 64 MiB, and a full-sync didChange carries one file plus its envelope.
const maxMessage = 80 << 20

// JSON-RPC 2.0 error codes.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
	codeServerNotInit  = -32002
)

// message is any JSON-RPC 2.0 message: a request (ID and Method), a
// notification (Method only) or a response (ID with Result or Error).
type message struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

func (m *message) isRequest() bool      { return m.ID != nil && m.Method != "" }
func (m *message) isNotification() bool { return m.ID == nil && m.Method != "" }

// rpcError is a JSON-RPC error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("jsonrpc %d: %s", e.Code, e.Message) }

// conn reads and writes Content-Length framed JSON-RPC messages. Reads
// happen on one goroutine; writes may come from any and are serialized.
type conn struct {
	r *bufio.Reader

	mu sync.Mutex
	w  io.Writer
}

func newConn(r io.Reader, w io.Writer) *conn {
	return &conn{r: bufio.NewReader(r), w: w}
}

// maxHeader bounds the header block of one message.
const maxHeader = 64 << 10

// read returns the next message. A malformed body is reported as a
// *rpcError (with the message when its id could be decoded) so the caller
// can answer it and go on; a broken frame or stream is a plain error that
// ends the connection.
func (c *conn) read() (*message, error) {
	n, err := c.readHeader()
	if err != nil {
		return nil, err
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(c.r, body); err != nil {
		return nil, fmt.Errorf("lsp: reading body: %w", err)
	}
	var m message
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, &rpcError{Code: codeParseError, Message: err.Error()}
	}
	if m.JSONRPC != "2.0" {
		return &m, &rpcError{Code: codeInvalidRequest, Message: `jsonrpc must be "2.0"`}
	}
	if m.Method != "" && m.ID != nil && !validID(*m.ID) {
		m.ID = nil
		return &m, &rpcError{Code: codeInvalidRequest, Message: "id must be a number or a string"}
	}
	return &m, nil
}

// readHeader reads the header block and returns the Content-Length.
func (c *conn) readHeader() (int, error) {
	length, total := -1, 0
	for {
		line, err := c.r.ReadSlice('\n')
		total += len(line)
		switch {
		case errors.Is(err, io.EOF) && total == 0:
			return 0, io.EOF
		case err != nil:
			return 0, fmt.Errorf("lsp: reading header: %w", err)
		case total > maxHeader:
			return 0, fmt.Errorf("lsp: header exceeds %d bytes", maxHeader)
		}
		text := strings.TrimRight(string(line), "\r\n")
		if text == "" {
			if length < 0 {
				return 0, errors.New("lsp: missing Content-Length")
			}
			return length, nil
		}
		name, value, _ := strings.Cut(text, ":")
		if !strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 0 {
			return 0, fmt.Errorf("lsp: bad Content-Length %q", strings.TrimSpace(value))
		}
		if n > maxMessage {
			return 0, fmt.Errorf("lsp: message of %d bytes exceeds the %d-byte limit", n, maxMessage)
		}
		length = n
	}
}

// validID reports whether a request id is a number or a string.
func validID(id json.RawMessage) bool {
	var v any
	if json.Unmarshal(id, &v) != nil {
		return false
	}
	switch v.(type) {
	case float64, string:
		return true
	}
	return false
}

// write sends m with its Content-Length header.
func (c *conn) write(m *message) error {
	m.JSONRPC = "2.0"
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = c.w.Write(body)
	return err
}

// reply answers the request id with result, or with err when it is set.
// An unknown id (a message that could not be decoded) is sent as null.
func (c *conn) reply(id *json.RawMessage, result any, err *rpcError) error {
	if id == nil {
		null := json.RawMessage("null")
		id = &null
	}
	m := &message{ID: id}
	if err != nil {
		m.Error = err
		return c.write(m)
	}
	raw, merr := json.Marshal(result)
	if merr != nil {
		m.Error = &rpcError{Code: codeInternalError, Message: merr.Error()}
		return c.write(m)
	}
	m.Result = raw
	return c.write(m)
}

// notify sends a notification.
func (c *conn) notify(method string, params any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return c.write(&message{Method: method, Params: raw})
}

// call sends a request whose response the server does not wait for; the
// reply arrives later on the read loop and is dropped.
func (c *conn) call(id int, method string, params any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	rid := json.RawMessage(strconv.Itoa(id))
	return c.write(&message{ID: &rid, Method: method, Params: raw})
}
