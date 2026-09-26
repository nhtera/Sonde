// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package curl

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// buildEntry turns one parsed curl command into an EntrySpec, or reports
// why it could not (no URL at all: the only case this package treats as
// fatal for the command, since a request needs one).
func buildEntry(p *parsed) (syntax.EntrySpec, []convert.Warning, string) {
	var warnings []convert.Warning
	warn := func(kind, msg string) { warnings = append(warnings, convert.Warning{Kind: kind, Message: msg}) }
	// warnRuntimeUnsupported is for a flag this package DOES map into
	// [Options] (so the file round-trips it faithfully, and a future
	// sonde can pick it up) but that internal/httpx.checkSupported
	// rejects at run time today (S1, phase 8 checkpoint): without this,
	// "sonde import curl" for one of these succeeds silently and the
	// resulting file only fails much later, when actually run. Wording
	// matches internal/convert/postman/auth.go's own unsupported-auth
	// warning.
	warnRuntimeUnsupported := func(flag string) {
		warn(convert.WarnUnsupportedOption, "sonde cannot send "+flag+" yet; this request fails until it can")
	}

	if len(p.urls) == 0 {
		return syntax.EntrySpec{}, nil, "curl: missing URL"
	}
	url, _ := pickURL(p.urls)
	if len(p.urls) > 1 {
		warn(convert.WarnUnsupported, "curl: only the first of "+strconv.Itoa(len(p.urls))+" URLs is imported ("+url.text()+")")
	}

	headers := make([]syntax.Field, 0, len(p.headers))
	var headerNames []string
	addHeader := func(name string, value templatedString) {
		headers = append(headers, syntax.Field{Key: syntax.PlainText(name), Value: value.toText()})
		headerNames = append(headerNames, name)
	}
	addHeaderText := func(name, value string) { addHeader(name, litString(value)) }
	hasHeader := func(name string) bool {
		for _, h := range headerNames {
			if strings.EqualFold(h, name) {
				return true
			}
		}
		return false
	}
	for _, h := range p.headers {
		addHeader(h.name, h.value)
	}

	var multipart []syntax.MultipartField
	for _, f := range p.form {
		if f.isFile {
			multipart = append(multipart, syntax.MultipartField{
				Key: syntax.PlainText(f.name),
				File: &syntax.MultipartFile{
					Name:        f.file.toText(),
					ContentType: f.contentType.toText(),
				},
			})
			continue
		}
		multipart = append(multipart, syntax.MultipartField{Key: syntax.PlainText(f.name), Value: f.value.toText()})
	}

	var body *syntax.BodySpec
	if len(multipart) == 0 {
		// --json is just another -d-family piece by the time it reaches
		// p.data (see apply's "json" case), so a real $TOKEN in either one
		// keeps working the same way: bodyLang picks the "json" fence only
		// for display, the actual body is otherwise built exactly like a
		// plain -d body (TextBody when it has a shell expansion, so the
		// {{name}} it renders isn't backtick-escaped away; RawTextBody
		// otherwise, so a literal "{{x}}" already in the data stays
		// literal — see buildDataBody's own doc comment).
		bodyLang := ""
		if p.haveJSON {
			bodyLang = "json"
		}
		var dataWarn string
		body, dataWarn = buildDataBody(p, bodyLang)
		if dataWarn != "" {
			warn(convert.WarnUnsupportedBody, dataWarn)
		}
	}
	hasBody := body != nil || len(multipart) > 0

	if p.get && len(multipart) > 0 {
		// curl itself rejects -G/--get together with -F/--form outright
		// (L2, phase 8 review); this package cannot fail the whole
		// command over an invalid flag combination the way curl would,
		// so it warns and otherwise ignores -G, keeping the multipart
		// body — the least surprising of the choices available here.
		warn(convert.WarnUnsupportedOption, "curl: -G/--get with -F/--form is invalid (curl itself rejects it); -G was ignored, the multipart body was kept")
	} else if p.get && hasBody {
		query, w := joinData(p.data)
		if w != "" {
			warn(convert.WarnUnsupportedBody, w)
		}
		if !query.isEmpty() {
			sep := "?"
			if url.contains("?") {
				sep = "&"
			}
			url = url.concat(sep).add(query)
		}
		body, hasBody = nil, false
	}

	// The --json headers are added only once it's settled whether there
	// is still a body at all: -G (above) can turn a --json command into
	// one with no body, and the implied Content-Type/Accept must not
	// survive that (L2, phase 8 review).
	if p.haveJSON && hasBody {
		if !hasHeader("Content-Type") && !p.suppressContentType {
			addHeaderText("Content-Type", "application/json")
		}
		if !hasHeader("Accept") && !p.suppressAccept {
			addHeaderText("Accept", "application/json")
		}
	}

	if hasBody && len(multipart) == 0 && !hasHeader("Content-Type") && !p.suppressContentType {
		addHeaderText("Content-Type", "application/x-www-form-urlencoded")
	}

	var cookies []syntax.Field
	if p.haveCookie {
		if !p.cookie.contains("=") {
			warn(convert.WarnUnsupportedOption, "curl: --cookie "+p.cookie.text()+" reads a cookie file, which is not supported; pass name=value pairs instead")
		} else {
			for _, part := range splitTemplated(p.cookie, ";") {
				part = part.trimSpace()
				if part.isEmpty() {
					continue
				}
				name, value, _ := part.cut("=")
				cookies = append(cookies, syntax.Field{Key: syntax.PlainText(name.trimSpace().text()), Value: value.trimSpace().toText()})
			}
		}
	}

	if p.haveUA && !hasHeader("User-Agent") {
		addHeader("User-Agent", p.userAgent)
	}
	if p.haveRef {
		if p.referer.text() == ";auto" {
			warn(convert.WarnUnsupportedOption, "curl: --referer ;auto (automatic Referer on redirect) is not supported")
		} else if !hasHeader("Referer") {
			addHeader("Referer", p.referer)
		}
	}

	var opts []syntax.OptionField
	// -u/--user maps to the [Options] "user" directive, not [BasicAuth]:
	// curl's own exporter (engine/curl.go) renders it back as --user too
	// (httpx.Options.User, not a header), and --user is also how -u
	// combines with --digest/--ntlm/--negotiate for a non-Basic scheme, a
	// combination [BasicAuth] (which always sends a literal Basic header)
	// cannot express at all.
	if p.haveUser {
		opts = append(opts, syntax.StringOption("user", p.user.toText()))
		if _, password, ok := p.user.cut(":"); ok && !password.isEmpty() && !password.hasExpansion() {
			// A literal password (not a $VAR the user is already
			// templating) written straight into the command line: flag it
			// without repeating the value itself (lead's S1 addendum,
			// phase 8 checkpoint).
			warn(convert.WarnSecret, "curl: -u wrote a password in plain text; consider --secret and {{password}}")
		}
	}
	if p.insecure {
		opts = append(opts, syntax.BoolOption("insecure", true))
	}
	if p.location {
		opts = append(opts, syntax.BoolOption("location", true))
	}
	if p.compressed {
		opts = append(opts, syntax.BoolOption("compressed", true))
	}
	if p.http10 {
		opts = append(opts, syntax.BoolOption("http1.0", true))
		warnRuntimeUnsupported("--http1.0")
	}
	if p.http11 {
		opts = append(opts, syntax.BoolOption("http1.1", true))
	}
	if p.http2 {
		opts = append(opts, syntax.BoolOption("http2", true))
	}
	if p.http3 {
		opts = append(opts, syntax.BoolOption("http3", true))
		warnRuntimeUnsupported("--http3")
	}
	if p.haveProxy {
		opts = append(opts, syntax.StringOption("proxy", p.proxy.toText()))
	}
	if p.haveConnTimeout {
		if d, ok := parseSeconds(p.connectTimeout); ok {
			opts = append(opts, syntax.DurationOption("connect-timeout", d))
		} else {
			warn(convert.WarnUnsupportedOption, "curl: --connect-timeout "+p.connectTimeout+" is not a number of seconds")
		}
	}
	if p.haveMaxTime {
		if d, ok := parseSeconds(p.maxTime); ok {
			opts = append(opts, syntax.DurationOption("max-time", d))
		} else {
			warn(convert.WarnUnsupportedOption, "curl: --max-time "+p.maxTime+" is not a number of seconds")
		}
	}
	if p.haveCACert {
		opts = append(opts, syntax.FilenameOption("cacert", p.cacert.toText()))
	}
	if p.haveCert {
		opts = append(opts, syntax.FilenameOption("cert", p.cert.toText()))
	}
	if p.haveKey {
		opts = append(opts, syntax.FilenameOption("key", p.key.toText()))
	}
	if p.locationTrusted {
		opts = append(opts, syntax.BoolOption("location-trusted", true))
	}
	if p.digest {
		opts = append(opts, syntax.BoolOption("digest", true))
		warnRuntimeUnsupported("--digest")
	}
	if p.ntlm {
		opts = append(opts, syntax.BoolOption("ntlm", true))
		warnRuntimeUnsupported("--ntlm")
	}
	if p.negotiate {
		opts = append(opts, syntax.BoolOption("negotiate", true))
		warnRuntimeUnsupported("--negotiate")
	}
	if p.haveAWSSigV4 {
		opts = append(opts, syntax.StringOption("aws-sigv4", p.awsSigV4.toText()))
		warnRuntimeUnsupported("--aws-sigv4")
	}
	if p.ipv4 {
		opts = append(opts, syntax.BoolOption("ipv4", true))
	}
	if p.ipv6 {
		opts = append(opts, syntax.BoolOption("ipv6", true))
	}
	if p.pathAsIs {
		opts = append(opts, syntax.BoolOption("path-as-is", true))
	}
	if p.haveUnixSocket {
		opts = append(opts, syntax.StringOption("unix-socket", p.unixSocket.toText()))
	}
	if p.haveOutput {
		opts = append(opts, syntax.FilenameOption("output", p.output.toText()))
	}
	if p.havePinnedKey {
		opts = append(opts, syntax.StringOption("pinnedpubkey", p.pinnedKey.toText()))
	}
	if p.haveMaxRedirs {
		if n, ok := parseMaxRedirs(p.maxRedirs); ok {
			opts = append(opts, syntax.IntOption("max-redirs", n))
		} else {
			warn(convert.WarnUnsupportedOption, "curl: --max-redirs "+p.maxRedirs+" is not a valid count")
		}
	}
	if p.haveLimitRate {
		if n, ok := parseLimitRate(p.limitRate); ok {
			opts = append(opts, syntax.IntOption("limit-rate", n))
		} else {
			warn(convert.WarnUnsupportedOption, "curl: --limit-rate "+p.limitRate+" is not a valid rate")
		}
	}
	for _, r := range p.resolve {
		opts = append(opts, syntax.StringOption("resolve", r.toText()))
	}
	for _, c := range p.connectTo {
		opts = append(opts, syntax.StringOption("connect-to", c.toText()))
	}

	for _, flag := range p.warnings {
		warn(convert.WarnUnsupportedOption, "curl: "+flag+" is not supported and was ignored")
	}

	return syntax.EntrySpec{
		Method:    resolveMethod(p, hasBody),
		URL:       url.toText(),
		Headers:   headers,
		Cookies:   cookies,
		Multipart: multipart,
		Options:   opts,
		Body:      body,
	}, warnings, ""
}

// pickURL chooses which of urls (parseArgv's collected positional
// arguments and --url values, in argv order) is the request's URL. This
// package already only ever builds one entry per curl command (a
// genuine multi-URL command, which curl runs as several separate
// requests, is out of scope — see buildEntry's own "only the first...
// is imported" warning), so the choice was always urls[0]; it now
// instead prefers the first entry with a recognizable "scheme://" prefix
// when one exists. That is a safety net for H3 (phase 8 review): a
// value-taking flag this package still doesn't recognize (despite
// ignoredValueFlags/longValueFlags being curated to be close to
// exhaustive) would otherwise have its argument silently become the
// request's target instead of the real URL a few positions later.
func pickURL(urls []templatedString) (url templatedString, index int) {
	for i, u := range urls {
		if hasScheme(u) {
			return u, i
		}
	}
	return urls[0], 0
}

// hasScheme reports whether s starts with a URL scheme ("https://",
// "custom-scheme+v2://", ...: RFC 3986's scheme grammar). It only looks
// at s's first literal run (see templatedString.firstLitOrEmpty): a word
// starting with a shell expansion could still be a URL once rendered, but
// this is only ever a tie-breaker among multiple candidates (see
// pickURL), not a validity check, so treating that case as "no scheme"
// and falling through to the plain first-argument choice is fine.
func hasScheme(s templatedString) bool {
	t := s.firstLitOrEmpty()
	if t == "" || (t[0] < 'a' || t[0] > 'z') && (t[0] < 'A' || t[0] > 'Z') {
		return false
	}
	i := 1
	for i < len(t) && isSchemeChar(t[i]) {
		i++
	}
	return strings.HasPrefix(t[i:], "://")
}

func isSchemeChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.'
}

// resolveMethod picks the request method: an explicit -X always wins,
// then -I (HEAD), then -G (GET, even with data: that is the point of
// -G), then POST when the command has a body, else GET.
func resolveMethod(p *parsed, hasBody bool) string {
	switch {
	case p.method != "":
		return p.method
	case p.head:
		return "HEAD"
	case p.get:
		return "GET"
	case hasBody:
		return "POST"
	default:
		return "GET"
	}
}

// buildDataBody joins every -d/--data-family (including --json: see
// apply's "json" case) piece into the request body: a single bare "@file"
// piece becomes a file reference; anything else is joined with '&' into
// one text body tagged with lang (buildEntry passes "json" when --json was
// used, "" otherwise — display only, e.g. a ```json fence). It returns a
// warning message when a piece could not be included (a file mixed with
// other pieces, or an unsupported --data-urlencode file form). A joined
// body with a $NAME expansion is built with syntax.TextBody, whose lang
// form is templated, so it becomes a real {{name}} reference; one with
// none goes through syntax.RawTextBody's untemplated form instead, so a
// literal "{{x}}" already in the data (no expansion involved at all)
// stays literal exactly as before.
func buildDataBody(p *parsed, lang string) (*syntax.BodySpec, string) {
	if len(p.data) == 0 {
		return nil, ""
	}
	if len(p.data) == 1 && p.data[0].kind == kindFile {
		return syntax.FileBody(p.data[0].text.toText()), ""
	}
	joined, warning := joinData(p.data)
	if joined.hasExpansion() {
		return syntax.TextBody(joined.toText(), lang), warning
	}
	return syntax.RawTextBody(joined.text(), lang), warning
}

// joinData joins every -d/--data-family piece with '&', as curl does; a
// piece whose content is not available (a file mixed with other pieces,
// or an unsupported --data-urlencode file form) contributes nothing and
// is named in the returned warning.
func joinData(pieces []dataPiece) (templatedString, string) {
	parts := make([]templatedString, 0, len(pieces))
	var skipped []string
	for _, d := range pieces {
		switch d.kind {
		case kindFile:
			parts = append(parts, templatedString{})
			skipped = append(skipped, "@"+d.text.text())
		case kindUnsupported:
			parts = append(parts, templatedString{})
			skipped = append(skipped, d.text.text())
		default:
			parts = append(parts, d.text)
		}
	}
	joined := joinTemplated(parts, "&")
	if len(skipped) == 0 {
		return joined, ""
	}
	return joined, "curl: file argument(s) not read, contributed as empty: " + strings.Join(skipped, ", ")
}

// parseSeconds parses curl's --connect-timeout/--max-time argument, a
// non-negative number of seconds (fractional allowed).
func parseSeconds(s string) (time.Duration, bool) {
	f, err := strconv.ParseFloat(s, 64)
	// strconv.ParseFloat itself accepts "inf"/"infinity"/"nan" (case
	// insensitive, optionally signed) as valid float syntax; converting
	// either to a Duration silently produces a huge or meaningless value
	// instead of failing (L3, phase 8 review), so both are rejected here
	// explicitly.
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return 0, false
	}
	seconds := f * float64(time.Second)
	if seconds > math.MaxInt64 {
		return 0, false
	}
	return time.Duration(seconds), true
}

// parseMaxRedirs parses curl's --max-redirs argument: an integer, -1 for
// unlimited (the [Options] "max-redirs" grammar itself accepts nothing
// smaller).
func parseMaxRedirs(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < -1 {
		return 0, false
	}
	return n, true
}

// parseLimitRate parses curl's --limit-rate argument: a plain number of
// bytes/second, or one with a trailing (case-insensitive) k/m/g suffix
// multiplying it by 1024/1024²/1024³, matching curl's own parsing.
func parseLimitRate(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	mul := int64(1)
	switch s[len(s)-1] {
	case 'k', 'K':
		mul, s = 1024, s[:len(s)-1]
	case 'm', 'M':
		mul, s = 1024*1024, s[:len(s)-1]
	case 'g', 'G':
		mul, s = 1024*1024*1024, s[:len(s)-1]
	}
	n, err := strconv.ParseInt(s, 10, 64)
	// n*mul below can overflow int64 for a large enough n even though n
	// itself parsed fine (L3, phase 8 review: "99999999999999999g"
	// silently wrapped around to a small positive number); reject it
	// instead of wrapping.
	if err != nil || n < 0 || (mul > 1 && n > math.MaxInt64/mul) {
		return 0, false
	}
	return n * mul, true
}
