// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import "encoding/json"

// The subset of LSP 3.17 types the server uses, with field names as the
// specification writes them.

// Position is a zero-based line and a character offset counted in the
// negotiated position encoding.
type Position struct {
	Line      uint32 `json:"line"`
	Character uint32 `json:"character"`
}

// Range is a half-open [Start, End) span.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Position encodings.
const (
	encodingUTF8  = "utf-8"
	encodingUTF16 = "utf-16"
)

// Text document sync kinds.
const syncFull = 1

// Diagnostic severities.
const (
	severityError       = 1
	severityWarning     = 2
	severityInformation = 3
)

// Diagnostic tags.
const tagDeprecated = 2

// Completion item kinds used by the server.
const (
	kindMethod   = 2
	kindFunction = 3
	kindField    = 5
	kindVariable = 6
	kindModule   = 9
	kindProperty = 10
	kindKeyword  = 14
	kindOperator = 24
)

// Insert text format for snippets.
const formatSnippet = 2

// Markup kinds.
const (
	markupPlainText = "plaintext"
	markupMarkdown  = "markdown"
)

// File change type in workspace/didChangeWatchedFiles.
const fileChanged = 2

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type versionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int32  `json:"version"`
}

type textDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int32  `json:"version"`
	Text       string `json:"text"`
}

type textDocumentPositionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type contentChange struct {
	Range *Range `json:"range,omitempty"`
	Text  string `json:"text"`
}

type didChangeParams struct {
	TextDocument   versionedTextDocumentIdentifier `json:"textDocument"`
	ContentChanges []contentChange                 `json:"contentChanges"`
}

type didCloseParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type workspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

type initializeParams struct {
	RootURI               *string            `json:"rootUri"`
	WorkspaceFolders      []workspaceFolder  `json:"workspaceFolders"`
	InitializationOptions json.RawMessage    `json:"initializationOptions"`
	Capabilities          clientCapabilities `json:"capabilities"`
}

// initOptions are the server settings a client passes at initialize and in
// workspace/didChangeConfiguration (under "sonde").
type initOptions struct {
	// Env names the sonde.yaml environment whose variables are defined.
	Env *string `json:"env"`
}

type clientCapabilities struct {
	General *struct {
		PositionEncodings []string `json:"positionEncodings"`
	} `json:"general"`
	Workspace *struct {
		DidChangeWatchedFiles *struct {
			DynamicRegistration bool `json:"dynamicRegistration"`
		} `json:"didChangeWatchedFiles"`
	} `json:"workspace"`
	TextDocument *struct {
		Completion *struct {
			CompletionItem *struct {
				SnippetSupport bool `json:"snippetSupport"`
			} `json:"completionItem"`
		} `json:"completion"`
		Hover *struct {
			ContentFormat []string `json:"contentFormat"`
		} `json:"hover"`
	} `json:"textDocument"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type initializeResult struct {
	Capabilities serverCapabilities `json:"capabilities"`
	ServerInfo   serverInfo         `json:"serverInfo"`
}

type serverCapabilities struct {
	PositionEncoding           string             `json:"positionEncoding"`
	TextDocumentSync           int                `json:"textDocumentSync"`
	CompletionProvider         *completionOptions `json:"completionProvider,omitempty"`
	HoverProvider              bool               `json:"hoverProvider"`
	DocumentFormattingProvider bool               `json:"documentFormattingProvider"`
	Workspace                  *struct {
		WorkspaceFolders struct {
			Supported           bool `json:"supported"`
			ChangeNotifications bool `json:"changeNotifications"`
		} `json:"workspaceFolders"`
	} `json:"workspace,omitempty"`
}

type completionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
}

// Diagnostic is one problem in a document.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Code     string `json:"code,omitempty"`
	Source   string `json:"source"`
	Message  string `json:"message"`
	Tags     []int  `json:"tags,omitempty"`
}

type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     *int32       `json:"version,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// MarkupContent is hover or documentation text.
type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// TextEdit replaces Range with NewText.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// CompletionItem is one completion proposal.
type CompletionItem struct {
	Label            string         `json:"label"`
	Kind             int            `json:"kind,omitempty"`
	Detail           string         `json:"detail,omitempty"`
	Documentation    *MarkupContent `json:"documentation,omitempty"`
	SortText         string         `json:"sortText,omitempty"`
	FilterText       string         `json:"filterText,omitempty"`
	InsertText       string         `json:"insertText,omitempty"`
	InsertTextFormat int            `json:"insertTextFormat,omitempty"`
	TextEdit         *TextEdit      `json:"textEdit,omitempty"`
	Tags             []int          `json:"tags,omitempty"`
}

type completionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

// Hover is the result of textDocument/hover.
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

type formattingParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type fileEvent struct {
	URI  string `json:"uri"`
	Type int    `json:"type"`
}

type didChangeWatchedFilesParams struct {
	Changes []fileEvent `json:"changes"`
}

type didChangeConfigurationParams struct {
	Settings json.RawMessage `json:"settings"`
}

type didChangeWorkspaceFoldersParams struct {
	Event struct {
		Added   []workspaceFolder `json:"added"`
		Removed []workspaceFolder `json:"removed"`
	} `json:"event"`
}

type registration struct {
	ID              string `json:"id"`
	Method          string `json:"method"`
	RegisterOptions any    `json:"registerOptions,omitempty"`
}

type registrationParams struct {
	Registrations []registration `json:"registrations"`
}

type unregistration struct {
	ID     string `json:"id"`
	Method string `json:"method"`
}

type unregistrationParams struct {
	// The specification misspells the field; clients expect it as written.
	Unregisterations []unregistration `json:"unregisterations"`
}

type fileSystemWatcher struct {
	GlobPattern string `json:"globPattern"`
}

type didChangeWatchedFilesRegistrationOptions struct {
	Watchers []fileSystemWatcher `json:"watchers"`
}
