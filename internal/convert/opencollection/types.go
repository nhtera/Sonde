// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package opencollection

import (
	"strconv"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// rawYAML holds a body's or a variable's "data"/"value" field undecoded:
// its shape depends on the sibling "type"/"secret" field, so it is
// interpreted afterwards (decodeBody, resolveVariableValue) rather than
// declared as one Go type.
type rawYAML = yaml.Node

// This file declares the subset of the OpenCollection 1.0.0 schema this
// importer understands (docs/decisions/0002-opencollection-mapping.md).
// Every struct is decoded with go.yaml.in/yaml/v3's default, non-strict
// mode, so a field not declared here (or a whole document from a future
// spec version) is silently ignored rather than rejected: "unknown
// fields/versions: warning, not failure" (phase-08-importers-exporters.md).

// description is the OpenCollection "Description" shape: a plain string,
// an {content, type} object, or absent. UnmarshalYAML accepts either form
// and keeps only the text.
type description string

func (d *description) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		*d = description(s)
		return nil
	}
	var obj struct {
		Content string `yaml:"content"`
	}
	if err := unmarshal(&obj); err != nil {
		return err
	}
	*d = description(obj.Content)
	return nil
}

// document is one YAML document at the root of a single-file collection,
// or of a directory's opencollection.yml. Always build one with
// decodeDocumentNode (decode.go), never a plain yaml.Unmarshal: that is
// what decodes Items one at a time — so one malformed item is skipped
// (its reason recorded in decodeErrors) rather than failing the whole
// document (mapping doc, "Pinned version": "unknown fields/versions:
// warning, not failure") — and what bounds "items" nesting depth against
// a crafted cyclic alias (maxItemDepth). Items has no yaml tag on
// purpose: a plain decode leaving it empty is a safer failure mode than
// silently reintroducing the recursion decodeDocumentNode exists to
// avoid.
type document struct {
	OpenCollection string      `yaml:"opencollection"`
	Info           rootInfo    `yaml:"info"`
	Config         collConfig  `yaml:"config"`
	Request        requestDefs `yaml:"request"`
	Docs           description `yaml:"docs"`
	Items          []item
	decodeErrors   []string
}

type rootInfo struct {
	Name    string `yaml:"name"`
	Summary string `yaml:"summary"`
	Version string `yaml:"version"`
}

// collConfig is the collection's "config" block. Proxy and Protobuf are
// declared only to detect presence (mapping doc, "Unsupported entirely"):
// neither has a Sonde equivalent.
type collConfig struct {
	Environments []environment `yaml:"environments"`
	Proxy        yaml.Node     `yaml:"proxy"`
	Protobuf     yaml.Node     `yaml:"protobuf"`
}

// requestDefs is the "request" defaults object: a collection's or a
// folder's inherited headers, auth and variables (mapping doc, "Auth" and
// "Variables and environments").
type requestDefs struct {
	Headers   []header   `yaml:"headers"`
	Auth      *auth      `yaml:"auth"`
	Variables []variable `yaml:"variables"`
}

// itemInfo is the "info" block every item (folder, http, graphql, grpc,
// websocket, script, app) carries.
type itemInfo struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
	// Seq is read with seqValue: a producer sometimes writes it as a
	// quoted number, which a plain "int" field would reject outright
	// (mapping doc, "Ordering" — a type mismatch here is ignored, not
	// fatal to the item).
	Seq yaml.Node `yaml:"seq"`
}

// seqValue reads Seq as a non-negative int (0, "unset", for anything else:
// absent, zero, negative, or not a number at all).
func (i itemInfo) seqValue() int {
	var n int
	if i.Seq.Decode(&n) == nil && n > 0 {
		return n
	}
	var s string
	if i.Seq.Decode(&s) == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// item is one entry of an "items" array: a folder (Items non-nil) or a
// leaf request (Type "http"/"graphql"; other leaf types are recognized
// only to be skipped, mapping doc "Item kinds"). Within a document or
// another item's own Items, build one with decodeItemNode (decode.go),
// not a plain decode — see document's own comment; Items keeps its
// "items" yaml tag here only so a directory-layout request file (one
// item, decoded directly by dirread.go's loadRequestFile with no
// sibling-isolation concern — mapping doc, "Input layouts") still
// captures a nested "items" key it has no real use for, rather than
// silently dropping it.
type item struct {
	Info         itemInfo      `yaml:"info"`
	Request      requestDefs   `yaml:"request"` // folder defaults, ignored on a leaf
	Docs         description   `yaml:"docs"`
	Items        []item        `yaml:"items"` // folder children
	HTTP         *httpBlock    `yaml:"http"`
	GraphQL      *graphqlBlock `yaml:"graphql"`
	Runtime      runtimeBlock  `yaml:"runtime"`
	Settings     settings      `yaml:"settings"`
	decodeErrors []string
}

// httpBlock is the "http" block of an http leaf item.
type httpBlock struct {
	Method  string   `yaml:"method"`
	URL     string   `yaml:"url"`
	Headers []header `yaml:"headers"`
	Params  []param  `yaml:"params"`
	Auth    *auth    `yaml:"auth"`
	Body    *body    `yaml:"body"`
}

// graphqlBlock is the "graphql" block of a graphql leaf item: same
// envelope as httpBlock, but its body is {query, variables} rather than
// the http body's discriminated union (mapping doc, "Item kinds").
type graphqlBlock struct {
	URL     string       `yaml:"url"`
	Headers []header     `yaml:"headers"`
	Params  []param      `yaml:"params"`
	Auth    *auth        `yaml:"auth"`
	Body    *graphqlBody `yaml:"body"`
}

type graphqlBody struct {
	Query     string `yaml:"query"`
	Variables string `yaml:"variables"`
}

// settings is a leaf item's "settings" block: none of it has a Sonde
// equivalent (mapping doc, "Request fields"), so every field is declared
// as a bare yaml.Node — only its presence is ever inspected, to produce a
// WarnUnsupportedOption — which also means a producer's actual value
// shape (an int, a quoted duration string, ...) never fails the item.
type settings struct {
	EncodeURL       yaml.Node `yaml:"encodeUrl"`
	Timeout         yaml.Node `yaml:"timeout"`
	FollowRedirects yaml.Node `yaml:"followRedirects"`
	MaxRedirects    yaml.Node `yaml:"maxRedirects"`
	OmitHeaders     yaml.Node `yaml:"omitHeaders"`
}

type header struct {
	Name     string      `yaml:"name"`
	Value    string      `yaml:"value"`
	Disabled bool        `yaml:"disabled"`
	Desc     description `yaml:"description"`
}

type param struct {
	Name     string `yaml:"name"`
	Value    string `yaml:"value"`
	Type     string `yaml:"type"` // "query" or "path"
	Disabled bool   `yaml:"disabled"`
}

// auth is a discriminated union on Type; "inherit" is also written as the
// bare YAML string "inherit" by some producers, handled by
// auth.UnmarshalYAML.
type auth struct {
	Type string `yaml:"type"`

	// basic, digest
	Username string `yaml:"username"`
	Password string `yaml:"password"`

	// bearer
	Token string `yaml:"token"`

	// apikey
	Key       string `yaml:"key"`
	Value     string `yaml:"value"`
	Placement string `yaml:"placement"` // "header" or "query"
}

func (a *auth) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		a.Type = s
		return nil
	}
	type plain auth
	return unmarshal((*plain)(a))
}

// isInherit reports whether a resolves to "keep looking at the enclosing
// scope" (mapping doc, "Auth"): no auth object, or the literal "inherit".
func (a *auth) isInherit() bool { return a == nil || a.Type == "" || a.Type == "inherit" }

// body is a discriminated union on Type (mapping doc, "Request fields").
type body struct {
	Type string `yaml:"-"`
	Data string `yaml:"-"` // json, xml, text, sparql: raw text; set by decodeBody

	// form-urlencoded
	FormData []formField `yaml:"-"`
	// multipart-form
	MultipartData []multipartField `yaml:"-"`
	// file
	FileData []fileField `yaml:"-"`

	// variants: only Data/FormData/MultipartData/FileData of the selected
	// variant is used; the rest are named in a warning.
	Variants []bodyVariant `yaml:"variants"`

	// rawData backs FormData/MultipartData/FileData: "data" is shaped
	// differently per Type, so it is decoded once as raw YAML nodes and
	// interpreted in body.go by decodeBody.
	rawData rawYAML
}

func (b *body) UnmarshalYAML(unmarshal func(any) error) error {
	type plain struct {
		Type     string        `yaml:"type"`
		Data     rawYAML       `yaml:"data"`
		Variants []bodyVariant `yaml:"variants"`
	}
	var p plain
	if err := unmarshal(&p); err != nil {
		return err
	}
	b.Type = p.Type
	b.Variants = p.Variants
	b.rawData = p.Data
	return decodeBody(b)
}

type bodyVariant struct {
	Title    string `yaml:"title"`
	Selected bool   `yaml:"selected"`
	Body     *body  `yaml:"body"`
}

type formField struct {
	Name     string `yaml:"name"`
	Value    string `yaml:"value"`
	Disabled bool   `yaml:"disabled"`
}

type multipartField struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"` // "text" or "file"
	Value       string `yaml:"value"`
	ContentType string `yaml:"contentType"`
	Disabled    bool   `yaml:"disabled"`
}

type fileField struct {
	FilePath    string `yaml:"filePath"`
	ContentType string `yaml:"contentType"`
	Selected    bool   `yaml:"selected"`
}

// runtimeBlock is the "runtime" block: scripts, declarative assertions and
// actions (mapping doc, "Scripts, tests and assertions").
type runtimeBlock struct {
	Variables  []variable  `yaml:"variables"`
	Scripts    []script    `yaml:"scripts"`
	Assertions []assertion `yaml:"assertions"`
	Actions    []action    `yaml:"actions"`
}

type script struct {
	Type string `yaml:"type"`
	Code string `yaml:"code"`
}

type assertion struct {
	Expression string `yaml:"expression"`
	Operator   string `yaml:"operator"`
	Value      string `yaml:"value"`
	Disabled   bool   `yaml:"disabled"`
}

// action is a "runtime.actions[]" entry (currently only "set-variable" in
// the schema): it runs relative to a live response, so it has no
// declarative-to-Sonde translation and is always kept as a comment.
type action struct {
	Type  string `yaml:"type"`
	Phase string `yaml:"phase"`
}

// variable is a plain Variable or a SecretVariable, told apart by Secret;
// a SecretVariable's schema shape has no value field to decode. Its
// UnmarshalYAML (decode.go) never fails: a variable entry too malformed
// to use at all decodes to a nameless zero value, filtered out wherever
// variables are folded together, rather than failing every sibling
// variable, environment or item around it.
type variable struct {
	Name     string
	Secret   bool
	Disabled bool
	Value    rawYAML
}

// environment is one "config.environments[]" entry, or the body of an
// environments/<name>.yml file (directory layout). ExternalSecrets,
// DotEnvFilePath, Extends and ClientCertificates are declared only to
// detect presence (mapping doc, "Variables and environments"): none has a
// Sonde equivalent.
type environment struct {
	Name               string     `yaml:"name"`
	Variables          []variable `yaml:"variables"`
	ExternalSecrets    yaml.Node  `yaml:"externalSecrets"`
	DotEnvFilePath     string     `yaml:"dotEnvFilePath"`
	Extends            string     `yaml:"extends"`
	ClientCertificates yaml.Node  `yaml:"clientCertificates"`
}
