// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpfile

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// document is one parsed .http file: its file-scope variable definitions,
// in the order they were declared, and its requests, in the order they
// appear.
type document struct {
	fileVars []fileVarDef
	requests []*request
}

// fileVarDef is one "@name = value" line, or a "# @prompt name" directive
// (rawValue "", prompt true), which declares a variable without a value.
type fileVarDef struct {
	name     string
	rawValue string
	prompt   bool
}

// scriptBlock is a pre-request or response handler script: never executed,
// only kept as a comment. Exactly one of external (a referenced file path)
// or inline (the script text between "{%" and "%}") is set.
type scriptBlock struct {
	external string
	inline   string
}

// rawKV is one "Key: Value" header line, unparsed.
type rawKV struct{ key, value string }

// bodyKind selects how a request's body was written.
type bodyKind int

// Body kinds.
const (
	// bodyText is an inline body, its text held verbatim.
	bodyText bodyKind = iota
	// bodyFileRef is "< path": the referenced file's content, sent as is,
	// matching Sonde's own file body exactly.
	bodyFileRef
	// bodyFileRefSubstituted is "<@ path": the referenced file's content,
	// with its own variable references substituted by the source tool
	// before sending (REST Client's "@" form; JetBrains documents only the
	// plain "<" form).
	bodyFileRefSubstituted
)

// rawBody is a request body before conversion.
type rawBody struct {
	kind bodyKind
	text string // bodyText
	path string // bodyFileRef, bodyFileRefSubstituted
}

// outputRedirect is a ">> path" or ">>! path" response redirection.
type outputRedirect struct {
	path      string
	overwrite bool
}

// request is one parsed request block.
type request struct {
	name     string
	comments []string // generic comments and unsupported directives, in source order

	preScript      *scriptBlock
	responseScript *scriptBlock
	outputFile     *outputRedirect

	unsupportedDirectives []string

	// hasTimeout/timeout and hasConnTimeout/connTimeout hold "@timeout"/
	// "@connection-timeout": JetBrains' idle timeout for new packets (not a
	// total request duration) and its connection timeout. Sonde has no idle
	// timeout option, so these map to "max-time"/"connect-timeout" as the
	// closest approximation, not an exact equivalent.
	hasTimeout     bool
	timeout        time.Duration
	hasConnTimeout bool
	connTimeout    time.Duration

	method  string
	url     string
	headers []rawKV
	body    *rawBody
}

var (
	// requestLineRE splits a request line into its first whitespace-run and
	// the rest; methodTokenRE then decides whether that first part is a
	// method (JetBrains/REST Client methods are written all-uppercase, e.g.
	// PROPFIND, GRAPHQL, WEBSOCKET, not just the fixed HTTP method list) or
	// part of a bare URL (GET is then assumed).
	requestLineRE = regexp.MustCompile(`^(\S+)\s+(\S.*)$`)
	methodTokenRE = regexp.MustCompile(`^[A-Z][A-Z-]*$`)
	httpVersionRE = regexp.MustCompile(`(?i)\s+HTTP/[0-9.]+\s*$`)
	fileVarLineRE = regexp.MustCompile(`^@([A-Za-z_][\w.-]*)\s*=\s*(.*)$`)
	directiveRE   = regexp.MustCompile(`^@([A-Za-z][A-Za-z-]*)(?:\s+(.*))?$`)
)

// parseDocument splits data into file variables and requests. It never
// panics on malformed input: anything it cannot make sense of either
// becomes a kept comment or is silently skipped.
func parseDocument(data []byte) *document {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimPrefix(text, string(rune(0xFEFF)))
	lines := strings.Split(text, "\n")

	doc := &document{}
	segStart := 0
	sepName := ""
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "###") {
			parseSegment(doc, lines[segStart:i], sepName)
			segStart = i + 1
			sepName = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ln), "###"))
		}
	}
	parseSegment(doc, lines[segStart:], sepName)
	return doc
}

// parseSegment parses the lines between two "###" separators (or the start
// or end of the file): leading file variables and comments/directives, at
// most one request, and its body/response tail. It appends any request
// found to doc.requests and any "@var = value" definitions found to
// doc.fileVars.
func parseSegment(doc *document, lines []string, sepName string) {
	req := &request{name: sepName}
	var comments []string

	i, ok := parsePreamble(doc, lines, req, &comments)
	if !ok {
		return
	}

	reqLine := strings.TrimSpace(lines[i])
	req.method, req.url = splitMethodAndURL(reqLine)
	i++
	i = parseURLContinuation(lines, i, req)
	i = parseHeaders(lines, i, req, &comments)

	body, respScript, out := parseBodyAndTail(lines[i:])
	req.body = body
	req.responseScript = respScript
	req.outputFile = out
	req.comments = comments
	doc.requests = append(doc.requests, req)
}

// splitMethodAndURL splits a request line into its method (uppercased) and
// URL: any all-uppercase leading token (PROPFIND, GRAPHQL, WEBSOCKET, ...,
// not just the fixed HTTP method list) followed by more content is the
// method; otherwise the whole line is a bare URL and the method is GET.
func splitMethodAndURL(line string) (method, url string) {
	if m := requestLineRE.FindStringSubmatch(line); m != nil {
		first := m[1]
		if methodTokenRE.MatchString(first) && !looksLikeURLStart(first) {
			return first, httpVersionRE.ReplaceAllString(strings.TrimSpace(m[2]), "")
		}
	}
	return "GET", httpVersionRE.ReplaceAllString(line, "")
}

// looksLikeURLStart reports whether s, the request line's first
// whitespace-separated token, looks like the start of a URL rather than a
// method: a scheme, an absolute path or a template placeholder. A method
// token never contains any of these.
func looksLikeURLStart(s string) bool {
	return strings.Contains(s, "://") || strings.HasPrefix(s, "/") || strings.HasPrefix(s, "{{")
}

// parsePreamble consumes file variables, comments/directives and an
// optional pre-request script until it finds the request line, returning
// its index and true, or false if the segment holds no request at all.
func parsePreamble(doc *document, lines []string, req *request, comments *[]string) (int, bool) {
	i := 0
	for i < len(lines) {
		t := strings.TrimSpace(lines[i])
		switch {
		case t == "":
			i++
		case fileVarLineRE.MatchString(t):
			m := fileVarLineRE.FindStringSubmatch(t)
			doc.fileVars = append(doc.fileVars, fileVarDef{name: m[1], rawValue: strings.TrimSpace(m[2])})
			i++
		case strings.HasPrefix(t, "#") || strings.HasPrefix(t, "//"):
			applyCommentLine(doc, req, comments, stripCommentMarker(t))
			i++
		case strings.HasPrefix(t, "<"):
			req.preScript, i = scanScriptBlock(lines, i)
		default:
			return i, true
		}
	}
	return 0, false
}

// stripCommentMarker removes a leading "#" or "//" and surrounding space.
func stripCommentMarker(t string) string {
	if rest, ok := strings.CutPrefix(t, "//"); ok {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(strings.TrimPrefix(t, "#"))
}

// applyCommentLine dispatches one comment body (marker already stripped):
// a recognized directive updates req or doc.fileVars (for "@prompt");
// anything else, recognized or not, is also kept verbatim as a comment
// (recognized directives with no Sonde equivalent are the exception: they
// leave no trace, matching their silent no-op behavior).
func applyCommentLine(doc *document, req *request, comments *[]string, body string) {
	name, arg, ok := parseDirective(body)
	if !ok {
		*comments = append(*comments, body)
		return
	}
	switch name {
	case "name":
		if arg != "" {
			req.name = arg
		}
	case "prompt":
		promptName, _, _ := strings.Cut(arg, " ")
		if promptName != "" {
			doc.fileVars = append(doc.fileVars, fileVarDef{name: promptName, prompt: true})
		}
	case "no-redirect", "no-cookie-jar", "no-log":
		// Recognized, with no Sonde option to set: Sonde does not follow
		// redirects by default, has no cross-request cookie jar to
		// disable, and has no per-request logging switch.
	case "timeout":
		if d, ok := parseTimeoutValue(arg); ok {
			req.hasTimeout, req.timeout = true, d
		} else {
			req.unsupportedDirectives = append(req.unsupportedDirectives, body)
			*comments = append(*comments, body)
		}
	case "connection-timeout":
		if d, ok := parseTimeoutValue(arg); ok {
			req.hasConnTimeout, req.connTimeout = true, d
		} else {
			req.unsupportedDirectives = append(req.unsupportedDirectives, body)
			*comments = append(*comments, body)
		}
	default:
		req.unsupportedDirectives = append(req.unsupportedDirectives, body)
		*comments = append(*comments, body)
	}
}

// parseTimeoutValue parses a "@timeout"/"@connection-timeout" value: a bare
// number in seconds by default, or with an explicit "ms", "s" or "m" suffix
// (JetBrains: "By default, the timeout values are in seconds, but you can
// add an explicit unit of time after the value"). A negative value, or one
// that overflows a time.Duration once multiplied by its unit, is rejected
// (ok = false) rather than silently wrapping or producing an invalid
// "max-time: -1s" that would fail the whole import.
func parseTimeoutValue(s string) (time.Duration, bool) {
	s = strings.TrimSpace(s)
	unit := time.Second
	switch {
	case strings.HasSuffix(s, "ms"):
		unit, s = time.Millisecond, strings.TrimSuffix(s, "ms")
	case strings.HasSuffix(s, "s"):
		unit, s = time.Second, strings.TrimSuffix(s, "s")
	case strings.HasSuffix(s, "m"):
		unit, s = time.Minute, strings.TrimSuffix(s, "m")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	if n > math.MaxInt64/int64(unit) {
		return 0, false
	}
	return time.Duration(n) * unit, true
}

// parseDirective splits a comment body into an "@name arg..." directive.
func parseDirective(body string) (name, arg string, ok bool) {
	m := directiveRE.FindStringSubmatch(body)
	if m == nil {
		return "", "", false
	}
	return m[1], strings.TrimSpace(m[2]), true
}

// scanScriptBlock reads a "<" or ">" prefixed script reference starting at
// lines[i]: either an external file ("< ./file.js") on one line, or an
// inline "{% ... %}" script that may span multiple lines. It returns the
// block and the index just past what it consumed.
func scanScriptBlock(lines []string, i int) (*scriptBlock, int) {
	t := strings.TrimSpace(lines[i])
	rest := strings.TrimSpace(t[1:])
	inline, ok := strings.CutPrefix(rest, "{%")
	if !ok {
		return &scriptBlock{external: rest}, i + 1
	}
	inline = strings.TrimSpace(inline)
	if end := strings.Index(inline, "%}"); end >= 0 {
		return &scriptBlock{inline: strings.TrimSpace(inline[:end])}, i + 1
	}
	var b strings.Builder
	if inline != "" {
		b.WriteString(inline)
		b.WriteByte('\n')
	}
	j := i + 1
	for j < len(lines) {
		if end := strings.Index(lines[j], "%}"); end >= 0 {
			b.WriteString(lines[j][:end])
			j++
			break
		}
		b.WriteString(lines[j])
		b.WriteByte('\n')
		j++
	}
	return &scriptBlock{inline: strings.TrimRight(b.String(), "\n")}, j
}

// parseURLContinuation appends JetBrains-style indented "?a=1"/"&b=2"
// continuation lines to req.url, returning the index of the first line that
// is not one.
func parseURLContinuation(lines []string, i int, req *request) int {
	for i < len(lines) {
		raw := lines[i]
		trimmed := strings.TrimLeft(raw, " \t")
		if trimmed == raw || trimmed == "" {
			break
		}
		if !strings.HasPrefix(trimmed, "?") && !strings.HasPrefix(trimmed, "&") {
			break
		}
		req.url += strings.TrimRight(trimmed, " \t")
		i++
	}
	return i
}

// parseHeaders consumes "Key: Value" lines (and interleaved comments) until
// a blank line, EOF or a line that is not a valid header, returning the
// index to resume parsing from.
func parseHeaders(lines []string, i int, req *request, comments *[]string) int {
	for i < len(lines) {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			return i + 1
		}
		if strings.HasPrefix(t, "#") || strings.HasPrefix(t, "//") {
			*comments = append(*comments, stripCommentMarker(t))
			i++
			continue
		}
		key, val, ok := splitHeader(t)
		if !ok {
			return i
		}
		req.headers = append(req.headers, rawKV{key, val})
		i++
	}
	return i
}

// splitHeader splits "Key: Value" at the first ':'.
func splitHeader(t string) (key, val string, ok bool) {
	idx := strings.Index(t, ":")
	if idx <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(t[:idx])
	if key == "" {
		return "", "", false
	}
	return key, strings.TrimSpace(t[idx+1:]), true
}

// tailKind classifies a body/tail line: 0 none, 1 a response handler
// (">..."), 2 an output redirect (">>..."), 3 an overwriting output
// redirect (">>!...").
func tailKind(t string) int {
	switch {
	case strings.HasPrefix(t, ">>!"):
		return 3
	case strings.HasPrefix(t, ">>"):
		return 2
	case strings.HasPrefix(t, ">"):
		return 1
	default:
		return 0
	}
}

// parseBodyAndTail splits the remainder of a request block into its body
// (if any), response handler script (if any) and output redirect (if any).
func parseBodyAndTail(lines []string) (*rawBody, *scriptBlock, *outputRedirect) {
	j := 0
	for j < len(lines) && tailKind(strings.TrimSpace(lines[j])) == 0 {
		j++
	}
	bodyLines := trimBlankEdges(lines[:j])

	var body *rawBody
	if len(bodyLines) > 0 {
		body = parseRawBody(bodyLines)
	}

	var resp *scriptBlock
	var out *outputRedirect
	tail := lines[j:]
	for k := 0; k < len(tail); {
		t := strings.TrimSpace(tail[k])
		if t == "" {
			k++
			continue
		}
		switch tailKind(t) {
		case 1:
			if resp != nil {
				k++
				continue
			}
			var consumed int
			resp, consumed = scanScriptBlock(tail, k)
			k = consumed
		case 2, 3:
			if out != nil {
				k++
				continue
			}
			marker := ">>"
			overwrite := tailKind(t) == 3
			if overwrite {
				marker = ">>!"
			}
			out = &outputRedirect{path: strings.TrimSpace(strings.TrimPrefix(t, marker)), overwrite: overwrite}
			k++
		default:
			k++
		}
	}
	return body, resp, out
}

// trimBlankEdges drops leading and trailing blank lines.
func trimBlankEdges(lines []string) []string {
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[start:end]
}

// parseRawBody builds a rawBody from a request's non-empty body lines: a
// "< path" or "<@ path" file reference spanning the whole body, or literal
// text otherwise.
func parseRawBody(lines []string) *rawBody {
	joined := strings.Join(lines, "\n")
	trimmed := strings.TrimSpace(joined)
	if rest, ok := strings.CutPrefix(trimmed, "<@"); ok {
		return &rawBody{kind: bodyFileRefSubstituted, path: strings.TrimSpace(rest)}
	}
	if rest, ok := strings.CutPrefix(trimmed, "<"); ok {
		return &rawBody{kind: bodyFileRef, path: strings.TrimSpace(rest)}
	}
	return &rawBody{kind: bodyText, text: joined}
}
