// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

// The AST is lossless: every byte of the source is held by some node —
// whitespace and comments as trivia (Whitespace, LineTerminator), literals
// with their source text next to the decoded value. Print(Parse(src)) == src.

// File is a parsed request file.
type File struct {
	// BOM is set when the source starts with a UTF-8 byte order mark, which
	// is skipped by the parser and kept by the printer.
	BOM     bool
	Entries []*Entry
	// LineTerminators are the blank and comment lines after the last entry.
	LineTerminators []*LineTerminator
}

// Entry is a request with its optional expected response.
type Entry struct {
	Request  *Request
	Response *Response
}

// Request is the request part of an entry.
type Request struct {
	LineTerminators []*LineTerminator
	Space0          Whitespace
	Method          Method
	Space1          Whitespace
	URL             *Template
	LineTerminator0 *LineTerminator
	Headers         []*KeyValue
	Sections        []*Section
	Body            *Body
	Span            Span
}

// Method is an HTTP method as written (uppercase letters).
type Method struct {
	Value string
	Span  Span
}

// Response is the expected response part of an entry.
type Response struct {
	LineTerminators []*LineTerminator
	Space0          Whitespace
	Version         Version
	Space1          Whitespace
	Status          Status
	LineTerminator0 *LineTerminator
	Headers         []*KeyValue
	Sections        []*Section
	Body            *Body
	Span            Span
}

// Version is the response HTTP version: "HTTP", "HTTP/1.0", "HTTP/1.1",
// "HTTP/2" or "HTTP/3".
type Version struct {
	Value string
	Span  Span
}

// Status is the expected status: "*" or digits as written.
type Status struct {
	Value string
	Span  Span
}

// Body is a request or response body.
type Body struct {
	LineTerminators []*LineTerminator
	Space0          Whitespace
	Value           Bytes
	Span            Span // the body value
	LineTerminator0 *LineTerminator
}

// Whitespace is a run of spaces/tabs (or newlines inside JSON-like values).
type Whitespace struct {
	Value string
	Span  Span
}

// LineTerminator is `sp* comment? newline?`; the newline is empty at EOF.
type LineTerminator struct {
	Space0  Whitespace
	Comment *Comment
	Newline Whitespace
}

// Comment is a `#` comment; Value excludes the `#`.
type Comment struct {
	Value string
	Span  Span
}

// KeyValue is `key: value` (headers, params, cookies, basic auth).
type KeyValue struct {
	LineTerminators []*LineTerminator
	Space0          Whitespace
	Key             *Template
	Space1          Whitespace
	Space2          Whitespace
	Value           *Template
	LineTerminator0 *LineTerminator
}

// SectionKind identifies a request or response section.
type SectionKind int

// Section kinds.
const (
	SectionQueryParams SectionKind = iota
	SectionFormParams
	SectionMultipart
	SectionCookies
	SectionBasicAuth
	SectionOptions
	SectionCaptures
	SectionAsserts
	SectionMessages // [SondeMessages], .sonde only
)

// Section is `[Name]` followed by its items. Only the slice matching Kind is
// set: KeyValues (query, form, cookies, basic auth — at most one), Multipart,
// Options, Captures, Asserts or Messages.
type Section struct {
	LineTerminators []*LineTerminator
	Space0          Whitespace
	Kind            SectionKind
	Name            string // as written, e.g. "Query" or "QueryStringParams"
	Span            Span   // the `[Name]` header
	LineTerminator0 *LineTerminator

	KeyValues []*KeyValue
	Multipart []MultipartParam
	Options   []*Option
	Captures  []*Capture
	Asserts   []*Assert
	Messages  []*MessageStep
}

// StepKind identifies a [SondeMessages] step; String returns its keyword.
type StepKind int

// Message step kinds.
const (
	StepSend StepKind = iota
	StepReceive
	StepClose
)

var stepNames = [...]string{"send", "receive", "close"}

func (k StepKind) String() string { return stepNames[k] }

// MessageStep is one step of a [SondeMessages] section: `send: body`,
// `receive`, `receive: N`, `close` or `close: CODE`. Colon is false for a
// bare `receive` or `close`, whose Space1, Space2 and Value are then empty.
type MessageStep struct {
	LineTerminators []*LineTerminator
	Space0          Whitespace
	Kind            StepKind
	Space1          Whitespace
	Colon           bool
	Space2          Whitespace
	// Value is a Bytes for send, a *Number or *Placeholder for receive and
	// close, nil without a value.
	Value           Node
	Span            Span // from the step keyword to the end of its value
	LineTerminator0 *LineTerminator
}

// MultipartParam is a *KeyValue or a *FilenameParam.
type MultipartParam interface{ multipartParam() }

func (*KeyValue) multipartParam()      {}
func (*FilenameParam) multipartParam() {}

// FilenameParam is `key: file,name; content-type`.
type FilenameParam struct {
	LineTerminators []*LineTerminator
	Space0          Whitespace
	Key             *Template
	Space1          Whitespace
	Space2          Whitespace
	Value           *FilenameValue
	LineTerminator0 *LineTerminator
}

// FilenameValue is `file,` space0 filename space1 `;` space2 content-type?.
type FilenameValue struct {
	Space0      Whitespace
	Filename    *Template
	Space1      Whitespace
	Space2      Whitespace
	ContentType *Template
}

// Capture is `name: query filters* redact?`.
type Capture struct {
	LineTerminators []*LineTerminator
	Space0          Whitespace
	Name            *Template
	Space1          Whitespace
	Space2          Whitespace
	Query           *Query
	Filters         []*FilterItem
	Space3          Whitespace // before `redact`, empty otherwise
	Redact          bool
	Span            Span // from the name to the last filter or `redact`
	LineTerminator0 *LineTerminator
}

// Assert is `query filters* predicate`.
type Assert struct {
	LineTerminators []*LineTerminator
	Space0          Whitespace
	Query           *Query
	Filters         []*FilterItem
	Space1          Whitespace
	Predicate       *Predicate
	Span            Span // from the query to the end of the predicate
	LineTerminator0 *LineTerminator
}

// FilterItem is a filter with the whitespace before it.
type FilterItem struct {
	Space  Whitespace
	Filter *Filter
}

// QueryKind identifies a query; String returns its keyword.
type QueryKind int

// Query kinds.
const (
	QueryStatus QueryKind = iota
	QueryVersion
	QueryURL
	QueryHeader // Arg: *Template
	QueryCookie // Arg: *CookiePath
	QueryBody
	QueryXPath    // Arg: *Template
	QueryJSONPath // Arg: *Template
	QueryRegex    // Arg: *Template or *Regex
	QueryVariable // Arg: *Template
	QueryDuration
	QueryBytes
	QueryRawBytes
	QuerySHA256
	QueryMD5
	QueryCertificate // Arg: *CertificateAttribute
	QueryIP
	QueryRedirects
	QuerySondeStream // Arg: nil or *StreamField; .sonde only
)

var queryNames = [...]string{"status", "version", "url", "header", "cookie", "body", "xpath",
	"jsonpath", "regex", "variable", "duration", "bytes", "rawbytes", "sha256", "md5",
	"certificate", "ip", "redirects", "sondeStream"}

func (k QueryKind) String() string { return queryNames[k] }

// Query extracts a value from the response. Queries with an argument have
// Space0 before it.
type Query struct {
	Span   Span
	Kind   QueryKind
	Space0 Whitespace
	Arg    Node
}

// CookiePath is a cookie query argument: `"name[Attribute]"`. Source is the
// text between the quotes; clear it after editing Name or Attribute.
type CookiePath struct {
	Source    string
	Name      *Template
	Attribute *CookieAttribute
}

// CookieAttribute is `[ Name ]` in a cookie path.
type CookieAttribute struct {
	Space0 Whitespace
	Name   string // as written; matched case-insensitively
	Space1 Whitespace
}

// StreamField is the quoted field of a sondeStream query, e.g. "event"
// (stored without quotes).
type StreamField struct {
	Name string
}

// CertificateAttribute is the quoted field of a certificate query, e.g.
// "Subject" (stored without quotes).
type CertificateAttribute struct {
	Name string
}

// FilterKind identifies a filter; String returns its keyword.
type FilterKind int

// Filter kinds.
const (
	FilterBase64Decode FilterKind = iota
	FilterBase64Encode
	FilterBase64URLSafeDecode
	FilterBase64URLSafeEncode
	FilterCharsetDecode // Arg: *Template
	FilterCount
	FilterDaysAfterNow
	FilterDaysBeforeNow
	FilterDecode // Arg: *Template (deprecated alias of charsetDecode)
	FilterFirst
	FilterFormat     // Arg: *Template (deprecated alias of dateFormat)
	FilterDateFormat // Arg: *Template
	FilterHTMLEscape
	FilterHTMLUnescape
	FilterJSONPath // Arg: *Template
	FilterLast
	FilterLocation
	FilterNth          // Arg: *Number or *Placeholder
	FilterRegex        // Arg: *Template or *Regex
	FilterReplace      // Arg, Arg2: *Template
	FilterReplaceRegex // Arg: *Template or *Regex; Arg2: *Template
	FilterSplit        // Arg: *Template
	FilterToDate       // Arg: *Template
	FilterToFloat
	FilterToHex
	FilterToInt
	FilterToString
	FilterURLDecode
	FilterURLEncode
	FilterURLQueryParam // Arg: *Template
	FilterUTF8Decode
	FilterUTF8Encode
	FilterXPath // Arg: *Template
)

var filterNames = [...]string{"base64Decode", "base64Encode", "base64UrlSafeDecode",
	"base64UrlSafeEncode", "charsetDecode", "count", "daysAfterNow", "daysBeforeNow", "decode",
	"first", "format", "dateFormat", "htmlEscape", "htmlUnescape", "jsonpath", "last",
	"location", "nth", "regex", "replace", "replaceRegex", "split", "toDate", "toFloat",
	"toHex", "toInt", "toString", "urlDecode", "urlEncode", "urlQueryParam", "utf8Decode",
	"utf8Encode", "xpath"}

func (k FilterKind) String() string { return filterNames[k] }

// Filter transforms a query result. Arguments are preceded by Space0/Space1.
type Filter struct {
	Span   Span
	Kind   FilterKind
	Space0 Whitespace
	Arg    Node
	Space1 Whitespace
	Arg2   Node
}

// Predicate is `not? func value?`.
type Predicate struct {
	Not    bool
	Space0 Whitespace // after `not`
	Func   *PredicateFunc
}

// PredicateKind identifies a predicate function; String returns its keyword.
type PredicateKind int

// Predicate kinds.
const (
	PredicateEqual PredicateKind = iota
	PredicateNotEqual
	PredicateGreater
	PredicateGreaterOrEqual
	PredicateLess
	PredicateLessOrEqual
	PredicateStartWith
	PredicateEndWith
	PredicateContain
	PredicateInclude
	PredicateMatch
	PredicateExist
	PredicateIsBoolean
	PredicateIsCollection
	PredicateIsDate
	PredicateIsEmpty
	PredicateIsFloat
	PredicateIsInteger
	PredicateIsIPv4
	PredicateIsIPv6
	PredicateIsIsoDate
	PredicateIsList
	PredicateIsNumber
	PredicateIsObject
	PredicateIsString
	PredicateIsUUID
)

var predicateNames = [...]string{"==", "!=", ">", ">=", "<", "<=", "startsWith", "endsWith",
	"contains", "includes", "matches", "exists", "isBoolean", "isCollection", "isDate",
	"isEmpty", "isFloat", "isInteger", "isIpv4", "isIpv6", "isIsoDate", "isList", "isNumber",
	"isObject", "isString", "isUuid"}

func (k PredicateKind) String() string { return predicateNames[k] }

// PredicateFunc is a predicate keyword with its optional value.
type PredicateFunc struct {
	Span   Span
	Kind   PredicateKind
	Space0 Whitespace
	Value  PredicateValue // nil for value-less predicates
}

// PredicateValue is one of *Null, *Boolean, *Number, *Template (quoted or
// backtick), *MultilineString, *Base64, *Hex, *FileRef, *Placeholder, *Regex.
type PredicateValue interface {
	Node
	predicateValue()
}

// Option is one `name: value` line in [Options].
type Option struct {
	LineTerminators []*LineTerminator
	Space0          Whitespace
	Name            string
	Space1          Whitespace
	Space2          Whitespace
	// Value is *Template (strings, filenames), *Boolean, *Number, *Duration,
	// *Placeholder, *VariableDefinition or *Identifier (verbosity).
	Value           Node
	LineTerminator0 *LineTerminator
}

// Duration is a natural number with an optional unit (ms, s, m, h).
type Duration struct {
	Value *Number
	Unit  string
}

// VariableDefinition is `name = value` in a variable option.
type VariableDefinition struct {
	Span   Span
	Name   string
	Space0 Whitespace
	Space1 Whitespace
	// Value is *Null, *Boolean, *Number or *Template.
	Value Node
}

// Identifier is a bare keyword value, e.g. the verbosity level.
type Identifier struct {
	Value string
}

// Bytes is a body value: a JSONValue, *XML, *MultilineString, *Template
// (backtick), *Base64, *Hex or *FileRef.
type Bytes interface {
	Node
	bytes()
}

// XML is an XML body kept verbatim.
type XML struct {
	Value string
}

// MultilineKind is the language of a multiline string.
type MultilineKind int

// Multiline string kinds.
const (
	MultilineText MultilineKind = iota
	MultilineJSON
	MultilineXML
	MultilineRaw
	MultilineGraphQL
)

// MultilineString is ```lang? ... ```.
type MultilineString struct {
	Kind MultilineKind
	// Lang is the language hint as written ("" for plain text).
	Lang string
	// Comma is set when a graphql hint is followed by the optional comma.
	Comma     bool
	Space     Whitespace
	Newline   Whitespace
	Value     *Template
	Variables *GraphQLVariables
}

// GraphQLVariables is the `variables {...}` block closing a graphql string.
type GraphQLVariables struct {
	Space      Whitespace
	Value      *JSONObject
	Whitespace Whitespace
}

// Base64 is `base64, ... ;`.
type Base64 struct {
	Space0 Whitespace
	Value  []byte
	Source string
	Space1 Whitespace
}

// Hex is `hex, ... ;`.
type Hex struct {
	Space0 Whitespace
	Value  []byte
	Source string
	Space1 Whitespace
}

// FileRef is `file, name ;`.
type FileRef struct {
	Space0   Whitespace
	Filename *Template
	Space1   Whitespace
}

// Regex is a /pattern/ literal. Source includes the slashes; Pattern has
// `\/` unescaped.
type Regex struct {
	Source  string
	Pattern string
}

// Template is a string made of literal parts and {{ }} placeholders.
// Delimiter is '"' or '`' for quoted strings, 0 when unquoted.
type Template struct {
	Delimiter rune
	Elements  []TemplateElement
	Span      Span
}

// TemplateElement is a *TemplateString or a *Placeholder.
type TemplateElement interface{ templateElement() }

// TemplateString is a literal part: decoded Value and its Source text
// (escaped as written). Print uses Source.
type TemplateString struct {
	Value  string
	Source string
}

// Placeholder is `{{ expr }}`.
type Placeholder struct {
	Space0 Whitespace
	Expr   Expr
	Space1 Whitespace
	// Trailing is text after the expression inside a string placeholder,
	// e.g. "b" in "{{a b}}". It is accepted and ignored for compatibility,
	// and kept for printing.
	Trailing string
}

// ExprKind distinguishes variables from functions.
type ExprKind int

// Expression kinds.
const (
	ExprVariable ExprKind = iota
	ExprFunction
)

// Expr is a variable name or a function (newDate, newUuid).
type Expr struct {
	Kind ExprKind
	Name string
	Span Span
}

// NumberKind distinguishes number representations.
type NumberKind int

// Number kinds.
const (
	NumberInteger NumberKind = iota
	NumberFloat
	NumberBigInteger
)

// Number is a numeric literal with its source text.
type Number struct {
	Kind   NumberKind
	Int    int64
	Float  float64
	Source string
}

// Boolean is `true` or `false`.
type Boolean struct {
	Value bool
}

// Null is `null`.
type Null struct{}

// JSONValue is *Null, *Boolean, *JSONNumber, *Template (string, delimiter
// '"'), *Placeholder, *JSONList or *JSONObject.
type JSONValue interface {
	Node
	jsonValue()
}

// JSONNumber is a JSON number as written.
type JSONNumber struct {
	Source string
}

// JSONList is `[ elements ]`; Space0 follows `[` when the list is empty.
type JSONList struct {
	Space0   string
	Elements []*JSONListElement
}

// JSONListElement is a list item with surrounding whitespace.
type JSONListElement struct {
	Space0 string
	Value  JSONValue
	Space1 string
}

// JSONObject is `{ members }`; Space0 follows `{` when the object is empty.
type JSONObject struct {
	Space0   string
	Elements []*JSONObjectElement
}

// JSONObjectElement is `"name": value` with surrounding whitespace.
type JSONObjectElement struct {
	Space0 string
	Name   *Template
	Space1 string
	Space2 string
	Value  JSONValue
	Space3 string
}

// Node is any AST value that can appear in a polymorphic position.
type Node interface{ node() }

func (*Template) node()             {}
func (*Placeholder) node()          {}
func (*Number) node()               {}
func (*Boolean) node()              {}
func (*Null) node()                 {}
func (*Regex) node()                {}
func (*Base64) node()               {}
func (*Hex) node()                  {}
func (*FileRef) node()              {}
func (*MultilineString) node()      {}
func (*XML) node()                  {}
func (*JSONNumber) node()           {}
func (*JSONList) node()             {}
func (*JSONObject) node()           {}
func (*Duration) node()             {}
func (*VariableDefinition) node()   {}
func (*Identifier) node()           {}
func (*CookiePath) node()           {}
func (*CertificateAttribute) node() {}
func (*StreamField) node()          {}

func (*TemplateString) templateElement() {}
func (*Placeholder) templateElement()    {}

func (*Null) predicateValue()            {}
func (*Boolean) predicateValue()         {}
func (*Number) predicateValue()          {}
func (*Template) predicateValue()        {}
func (*MultilineString) predicateValue() {}
func (*Base64) predicateValue()          {}
func (*Hex) predicateValue()             {}
func (*FileRef) predicateValue()         {}
func (*Placeholder) predicateValue()     {}
func (*Regex) predicateValue()           {}

func (*Null) bytes()            {}
func (*Boolean) bytes()         {}
func (*JSONNumber) bytes()      {}
func (*Template) bytes()        {}
func (*Placeholder) bytes()     {}
func (*JSONList) bytes()        {}
func (*JSONObject) bytes()      {}
func (*XML) bytes()             {}
func (*MultilineString) bytes() {}
func (*Base64) bytes()          {}
func (*Hex) bytes()             {}
func (*FileRef) bytes()         {}

func (*Null) jsonValue()        {}
func (*Boolean) jsonValue()     {}
func (*JSONNumber) jsonValue()  {}
func (*Template) jsonValue()    {}
func (*Placeholder) jsonValue() {}
func (*JSONList) jsonValue()    {}
func (*JSONObject) jsonValue()  {}
