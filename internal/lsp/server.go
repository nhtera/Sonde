// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Package lsp implements `sonde lsp`, a Language Server Protocol server over
// stdio for .hurl and .sonde files: diagnostics, completion, hover and
// formatting. It never sends HTTP requests and reads files only inside the
// workspace folders.
package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"

	"github.com/nhtera/sonde/internal/config"
	"github.com/nhtera/sonde/internal/docs"
)

// Options configures a Server.
type Options struct {
	// Version is reported in serverInfo.
	Version string
	// Environ is the process environment: SONDE_ENV selects the default
	// environment and HURL_VARIABLE_*/SONDE_VARIABLE_* define variables.
	Environ config.Env
}

// ErrExitWithoutShutdown is returned by Run when the client sent exit
// without shutdown first; the process should exit with status 1.
var ErrExitWithoutShutdown = errors.New("lsp: exit without shutdown")

// Server is a language server bound to one client connection. Messages are
// handled one at a time in arrival order.
type Server struct {
	opt   Options
	conn  *conn
	table *docs.Table
	docs  map[string]*document

	initialized bool
	shutdown    bool

	// Negotiated at initialize.
	utf16        bool
	snippets     bool
	markdown     bool
	dynamicWatch bool

	folders []string // workspace folder paths
	env     *string  // environment from the client settings, nil when unset
	// extraVariables are the names the client settings define (see
	// initOptions.ExtraVariables).
	extraVariables []string
	config         *configCache
	nextID         int
}

// NewServer returns a server; call Run to serve a connection.
func NewServer(opt Options) (*Server, error) {
	table, err := docs.Load()
	if err != nil {
		return nil, err
	}
	if opt.Environ == nil {
		opt.Environ = config.Env{}
	}
	s := &Server{opt: opt, table: table, docs: map[string]*document{}, utf16: true}
	s.config = newConfigCache(s)
	return s, nil
}

// Run serves the client on in/out until the client sends exit, the input
// ends or ctx is cancelled. It returns nil after a clean shutdown and exit.
func (s *Server) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	s.conn = newConn(in, out)
	type result struct {
		m   *message
		err error
	}
	msgs := make(chan result)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			m, err := s.conn.read()
			select {
			case msgs <- result{m, err}:
			case <-done:
				return
			}
			var rerr *rpcError
			if err != nil && !errors.As(err, &rerr) {
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case r := <-msgs:
			if r.err != nil {
				var rerr *rpcError
				if errors.As(r.err, &rerr) {
					var id *json.RawMessage
					if r.m != nil && r.m.Method != "" {
						id = r.m.ID
					}
					_ = s.conn.reply(id, nil, rerr)
					continue
				}
				if errors.Is(r.err, io.EOF) {
					return nil
				}
				return r.err
			}
			if r.m.Method == "exit" {
				if !s.shutdown {
					return ErrExitWithoutShutdown
				}
				return nil
			}
			s.handle(r.m)
		}
	}
}

// handle dispatches one message. Responses to the server's own requests
// (client/registerCapability) are ignored.
// A panic while handling is recovered: a request gets an internal error
// (without details, which could quote the document) and the server goes on.
func (s *Server) handle(m *message) {
	defer func() {
		if recover() != nil && m.isRequest() {
			_ = s.conn.reply(m.ID, nil, &rpcError{Code: codeInternalError, Message: "internal error handling " + m.Method})
		}
	}()
	switch {
	case m.isRequest():
		result, err := s.request(m)
		_ = s.conn.reply(m.ID, result, err)
	case m.isNotification():
		if s.initialized && !s.shutdown {
			s.notification(m)
		}
	}
}

func (s *Server) request(m *message) (any, *rpcError) {
	if m.Method == "initialize" {
		if s.initialized {
			return nil, &rpcError{Code: codeInvalidRequest, Message: "initialize sent twice"}
		}
		var p initializeParams
		if err := unmarshalParams(m.Params, &p); err != nil {
			return nil, err
		}
		return s.initialize(&p), nil
	}
	if !s.initialized {
		return nil, &rpcError{Code: codeServerNotInit, Message: "server not initialized"}
	}
	if s.shutdown {
		return nil, &rpcError{Code: codeInvalidRequest, Message: "server is shutting down"}
	}
	switch m.Method {
	case "shutdown":
		s.shutdown = true
		return nil, nil
	case "textDocument/completion":
		var p textDocumentPositionParams
		if err := unmarshalParams(m.Params, &p); err != nil {
			return nil, err
		}
		doc := s.docs[p.TextDocument.URI]
		if doc == nil {
			return nil, nil
		}
		return completionList{Items: s.completion(doc, doc.lines.offset(p.Position))}, nil
	case "textDocument/hover":
		var p textDocumentPositionParams
		if err := unmarshalParams(m.Params, &p); err != nil {
			return nil, err
		}
		doc := s.docs[p.TextDocument.URI]
		if doc == nil {
			return nil, nil
		}
		if h := s.hover(doc, doc.lines.offset(p.Position)); h != nil {
			return h, nil
		}
		return nil, nil
	case "textDocument/formatting":
		var p formattingParams
		if err := unmarshalParams(m.Params, &p); err != nil {
			return nil, err
		}
		doc := s.docs[p.TextDocument.URI]
		if doc == nil {
			return nil, nil
		}
		return s.format(doc), nil
	}
	return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + m.Method}
}

func (s *Server) notification(m *message) {
	switch m.Method {
	case "initialized":
		s.registerWatchers()
	case "textDocument/didOpen":
		var p didOpenParams
		if unmarshalParams(m.Params, &p) != nil {
			return
		}
		d := newDocument(p.TextDocument.URI, p.TextDocument.Version, p.TextDocument.Text, s.utf16)
		s.docs[d.uri] = d
		s.publish(d)
	case "textDocument/didChange":
		var p didChangeParams
		if unmarshalParams(m.Params, &p) != nil || len(p.ContentChanges) == 0 {
			return
		}
		d := s.docs[p.TextDocument.URI]
		if d == nil {
			return
		}
		// Full sync: the last change holds the whole text. A ranged change
		// breaks the negotiated sync kind and is ignored.
		last := p.ContentChanges[len(p.ContentChanges)-1]
		if last.Range != nil {
			return
		}
		d.version = p.TextDocument.Version
		d.setText(last.Text, s.utf16)
		s.publish(d)
	case "textDocument/didClose":
		var p didCloseParams
		if unmarshalParams(m.Params, &p) != nil {
			return
		}
		if _, ok := s.docs[p.TextDocument.URI]; ok {
			delete(s.docs, p.TextDocument.URI)
			_ = s.conn.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{URI: p.TextDocument.URI, Diagnostics: []Diagnostic{}})
		}
	case "workspace/didChangeWatchedFiles":
		var p didChangeWatchedFilesParams
		if unmarshalParams(m.Params, &p) != nil {
			return
		}
		s.config.invalidate(p.Changes)
		s.publishAll()
	case "workspace/didChangeConfiguration":
		var p didChangeConfigurationParams
		if unmarshalParams(m.Params, &p) != nil {
			return
		}
		var settings struct {
			Sonde *initOptions `json:"sonde"`
		}
		if json.Unmarshal(p.Settings, &settings) == nil && settings.Sonde != nil {
			s.env = settings.Sonde.Env
			s.extraVariables = settings.Sonde.ExtraVariables
			s.publishAll()
		}
	case "workspace/didChangeWorkspaceFolders":
		var p didChangeWorkspaceFoldersParams
		if unmarshalParams(m.Params, &p) != nil {
			return
		}
		for _, f := range p.Event.Removed {
			if path := uriToPath(f.URI); path != "" {
				s.folders = slices.DeleteFunc(s.folders, func(x string) bool { return x == path })
			}
		}
		for _, f := range p.Event.Added {
			if path := uriToPath(f.URI); path != "" {
				s.folders = append(s.folders, path)
			}
		}
		s.config.invalidate(nil)
		s.publishAll()
	}
}

func (s *Server) initialize(p *initializeParams) initializeResult {
	s.initialized = true
	s.utf16 = true
	if c := p.Capabilities.General; c != nil && slices.Contains(c.PositionEncodings, encodingUTF8) {
		s.utf16 = false
	}
	if c := p.Capabilities.TextDocument; c != nil {
		if c.Completion != nil && c.Completion.CompletionItem != nil {
			s.snippets = c.Completion.CompletionItem.SnippetSupport
		}
		if c.Hover != nil {
			s.markdown = slices.Contains(c.Hover.ContentFormat, markupMarkdown)
		}
	}
	if c := p.Capabilities.Workspace; c != nil && c.DidChangeWatchedFiles != nil {
		s.dynamicWatch = c.DidChangeWatchedFiles.DynamicRegistration
	}
	for _, f := range p.WorkspaceFolders {
		if path := uriToPath(f.URI); path != "" {
			s.folders = append(s.folders, path)
		}
	}
	if len(s.folders) == 0 && p.RootURI != nil {
		if path := uriToPath(*p.RootURI); path != "" {
			s.folders = append(s.folders, path)
		}
	}
	var opts initOptions
	if len(p.InitializationOptions) > 0 && json.Unmarshal(p.InitializationOptions, &opts) == nil {
		s.env = opts.Env
		s.extraVariables = opts.ExtraVariables
	}

	caps := serverCapabilities{
		PositionEncoding:           encodingUTF8,
		TextDocumentSync:           syncFull,
		CompletionProvider:         &completionOptions{TriggerCharacters: completionTriggers},
		HoverProvider:              true,
		DocumentFormattingProvider: true,
	}
	if s.utf16 {
		caps.PositionEncoding = encodingUTF16
	}
	caps.Workspace = &struct {
		WorkspaceFolders struct {
			Supported           bool `json:"supported"`
			ChangeNotifications bool `json:"changeNotifications"`
		} `json:"workspaceFolders"`
	}{}
	caps.Workspace.WorkspaceFolders.Supported = true
	caps.Workspace.WorkspaceFolders.ChangeNotifications = true
	return initializeResult{Capabilities: caps, ServerInfo: serverInfo{Name: "sonde", Version: s.opt.Version}}
}

// publish sends d's diagnostics.
func (s *Server) publish(d *document) {
	v := d.version
	_ = s.conn.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{
		URI: d.uri, Version: &v, Diagnostics: s.diagnostics(d),
	})
}

// publishAll re-sends diagnostics for every open document, e.g. after a
// configuration file changed.
func (s *Server) publishAll() {
	uris := make([]string, 0, len(s.docs))
	for uri := range s.docs {
		uris = append(uris, uri)
	}
	slices.Sort(uris)
	for _, uri := range uris {
		s.publish(s.docs[uri])
	}
}

// call sends a server-to-client request whose reply is ignored.
func (s *Server) call(method string, params any) {
	s.nextID++
	_ = s.conn.call(s.nextID, method, params)
}

func unmarshalParams(raw json.RawMessage, v any) *rpcError {
	if len(raw) == 0 {
		return &rpcError{Code: codeInvalidParams, Message: "missing params"}
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return &rpcError{Code: codeInvalidParams, Message: err.Error()}
	}
	return nil
}
