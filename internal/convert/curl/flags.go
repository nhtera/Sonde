// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package curl

import "strings"

// dataKind distinguishes the three ways a -d/--data-family argument ends
// up contributing to the body.
type dataKind int

const (
	// kindRaw is a literal or already percent-encoded string, joined with
	// '&' into the final body text.
	kindRaw dataKind = iota
	// kindFile is a bare "@path" argument to -d/--data-raw/--data-binary:
	// when it is the command's only data piece it becomes a `file,` body
	// reference; the file is never read by this package.
	kindFile
	// kindUnsupported is a --data-urlencode file form ("@file",
	// "name@file"): curl would read and encode the file, which this
	// package cannot do without reading untrusted paths, so the piece
	// contributes nothing and is reported as a warning.
	kindUnsupported
)

// dataPiece is one -d/--data/--data-raw/--data-binary/--data-urlencode
// argument, in command line order.
type dataPiece struct {
	kind dataKind
	// text is the piece's contribution to the joined body (already
	// percent-encoded for a --data-urlencode piece) for kindRaw, or the
	// file path (without its leading '@') for kindFile and
	// kindUnsupported.
	text templatedString
}

// formField is one -F/--form argument. name is always plain text (a
// field name templated from a shell variable is vanishingly rare, and
// this package flattens one to inert "{{name}}"-looking literal text
// rather than pretending to support it); value, file and contentType
// keep any $NAME/${NAME} expansion they had.
type formField struct {
	name string
	// value is the field's text value; file and contentType are set
	// instead for an "@file" or "@file;type=..." value.
	value       templatedString
	file        templatedString
	contentType templatedString
	isFile      bool
}

// kv is one -H header. name is always plain text (see formField.name);
// remove distinguishes curl's two special empty forms: "Name:" (nothing
// at all after the colon) removes the header — including one curl, or
// engine/curl.go's own exporter, would otherwise add automatically, such
// as the implicit Content-Type of a -d body — while "Name;" (no colon, a
// semicolon instead) sends the header with an empty value. See
// parseHeaderArg.
type kv struct {
	name   string
	value  templatedString
	remove bool
}

// parsed collects one curl command's flags before buildEntry (entry.go)
// turns them into request-file constructs.
type parsed struct {
	urls []templatedString // positional arguments and --url values, in order

	headers []kv // -H, in order
	data    []dataPiece
	form    []formField

	method string // -X/--request, flattened; "" means none given
	head   bool   // -I/--head
	get    bool   // -G/--get

	haveUser   bool
	user       templatedString
	haveCookie bool
	cookie     templatedString
	haveUA     bool
	userAgent  templatedString
	haveRef    bool
	referer    templatedString

	insecure        bool
	location        bool
	locationTrusted bool
	compressed      bool
	http11          bool
	http2           bool
	http3           bool
	digest          bool
	ntlm            bool
	negotiate       bool
	ipv4            bool
	ipv6            bool
	pathAsIs        bool
	http10          bool

	haveProxy       bool
	proxy           templatedString
	haveConnTimeout bool
	connectTimeout  string // flattened: parsed as a number of seconds
	haveMaxTime     bool
	maxTime         string // flattened: parsed as a number of seconds
	haveCACert      bool
	cacert          templatedString
	haveCert        bool
	cert            templatedString
	haveKey         bool
	key             templatedString
	haveUnixSocket  bool
	unixSocket      templatedString
	haveOutput      bool
	output          templatedString
	havePinnedKey   bool
	pinnedKey       templatedString
	haveMaxRedirs   bool
	maxRedirs       string // flattened: parsed as a count
	haveLimitRate   bool
	limitRate       string            // flattened: parsed as a byte rate
	resolve         []templatedString // --resolve, in order (repeatable)
	connectTo       []templatedString // --connect-to, in order (repeatable)

	haveAWSSigV4 bool
	awsSigV4     templatedString // --aws-sigv4: mapped (S1, phase 8 checkpoint), but httpx rejects it at run time — see buildEntry's warnRuntimeUnsupported

	haveJSON            bool // --json used at least once: see buildEntry's implied Content-Type/Accept
	suppressContentType bool // -H 'Content-Type:' (curl's "remove" form)
	suppressAccept      bool // -H 'Accept:' (curl's "remove" form)

	warnings []string // unsupported flag names, in first-seen order (deduped)
	seenWarn map[string]bool
}

func (p *parsed) warn(flag string) {
	if p.seenWarn == nil {
		p.seenWarn = map[string]bool{}
	}
	if !p.seenWarn[flag] {
		p.seenWarn[flag] = true
		p.warnings = append(p.warnings, flag)
	}
}

// booleanFlags are the recognized flags that never take a value, mapped
// from their long form (short forms are resolved to a long form first).
// A flag whose own case in apply already sets a field is listed here too,
// harmlessly, since apply never falls through to the default case for
// those names; every other entry is silently ignored: it only affects
// transport diagnostics, retry policy or an authentication scheme sonde
// does not support, never the rendered request itself.
var booleanFlags = map[string]bool{
	"insecure": true, "location": true, "location-trusted": true, "compressed": true,
	"http1.1": true, "http2": true, "http3": true, "get": true, "head": true,
	"digest": true, "ntlm": true, "negotiate": true, "ipv4": true, "ipv6": true,
	"path-as-is": true,
	"silent":     true, "show-error": true, "verbose": true, "include": true,
	"fail": true, "fail-early": true, "fail-with-body": true,
	"progress-bar": true, "remote-name": true, "disable": true,
	"globoff": true, "no-buffer": true, "no-keepalive": true,
	"http0.9": true,
	"tlsv1":   true, "tlsv1.0": true, "tlsv1.1": true, "tlsv1.2": true, "tlsv1.3": true,
	"basic": true, "anyauth": true, "http1.0": true,
	"netrc": true, "netrc-optional": true, "parallel": true,
	"retry-all-errors": true, "retry-connrefused": true,
	"tr-encoding": true, "tcp-nodelay": true, "trace-time": true,
	// Added for H3 (phase 8 review), alongside the short flags that map
	// to them: real curl short options with no argument of their own.
	"remote-header-name": true, "use-ascii": true, "manual": true,
	"proxy-tunnel": true, "append": true, "junk-session-cookies": true,
	"list-only": true, "remote-time": true, "sslv2": true, "sslv3": true,
}

// ignoredValueFlags are recognized value-taking flags with no Sonde
// equivalent that must still have their argument skipped so it is never
// mistaken for the URL. Reported with WarnUnsupportedOption. A flag here
// that does map to a real [Options] entry (netrc-file, ...) is still
// listed if it wasn't worth the added surface for this pass — see the
// phase report (aws-sigv4 used to be one of these too, but is now mapped:
// see longValueFlags and buildEntry's warnRuntimeUnsupported, S1 in the
// phase 8 checkpoint). This list is curated from curl's own option reference
// (curl --help all) to be close to exhaustive for every flag that takes a
// value (H3, phase 8 review): the goal is that no value-taking flag this
// package doesn't otherwise recognize can ever leak its argument into
// p.urls. buildEntry's own scheme-preferring pick of p.urls[0] is a
// second, independent safety net for anything still missing here.
var ignoredValueFlags = map[string]bool{
	"write-out": true, "retry": true,
	"retry-delay": true, "retry-max-time": true,
	"max-filesize": true, "interface": true, "dns-servers": true,
	"dns-interface": true, "local-port": true, "range": true,
	"time-cond": true, "stderr": true,
	"trace": true, "trace-ascii": true, "libcurl": true, "config": true,
	"cookie-jar": true, "proxy-user": true, "proxy-header": true,
	"noproxy": true, "socks5": true, "socks4": true, "socks4a": true,
	"socks5-hostname": true, "cert-type": true, "key-type": true,
	"pass": true, "engine": true, "egd-file": true, "random-file": true,
	"tlsauthtype": true, "tlspassword": true, "tlsuser": true,
	"login-options": true, "mail-from": true, "mail-rcpt": true,
	"netrc-file": true, "oauth2-bearer": true,
	"upload-file": true, "ciphers": true,
	"tls13-ciphers": true, "keepalive-time": true, "expect100-timeout": true,
	"happy-eyeballs-timeout-ms": true, "parallel-max": true,

	// Added for H3 (phase 8 review): every other value-taking flag in
	// curl's own option list this package has no better mapping for.
	"dump-header": true, "url-query": true, "speed-limit": true,
	"speed-time": true, "continue-at": true, "quote": true,
	"telnet-option": true, "ftp-port": true, "output-dir": true,
	"capath": true, "crlfile": true, "ftp-account": true,
	"ftp-alternate-to-user": true, "ftp-ssl-ccc-mode": true, "krb": true,
	"mail-auth": true, "proto": true, "proto-default": true,
	"proto-redir": true, "proxy-ca-native": true, "proxy-cacert": true,
	"proxy-cert": true, "proxy-cert-type": true, "proxy-ciphers": true,
	"proxy-crlfile": true, "proxy-key": true, "proxy-key-type": true,
	"proxy-pass": true, "proxy-pinnedpubkey": true, "proxy-service-name": true,
	"proxy-tls13-ciphers": true, "proxy-tlsauthtype": true,
	"proxy-tlspassword": true, "proxy-tlsuser": true, "pubkey": true,
	"request-target": true, "service-name": true,
	"socks5-gssapi-service": true, "tftp-blksize": true, "doh-url": true,
	"abstract-unix-socket": true, "haproxy-clientip": true,
	"hostpubmd5": true, "hostpubsha256": true,
}

// longValueFlags are the long-only flags this parser understands and
// applies, as taking a value.
var longValueFlags = map[string]bool{
	"request": true, "header": true, "data": true, "data-raw": true,
	"data-binary": true, "data-ascii": true, "data-urlencode": true,
	"json": true, "form": true, "form-string": true, "user": true, "cookie": true,
	"user-agent": true, "referer": true, "proxy": true, "connect-timeout": true,
	"max-time": true, "cacert": true, "cert": true, "key": true, "url": true,
	"unix-socket": true, "output": true, "pinnedpubkey": true,
	"max-redirs": true, "limit-rate": true, "resolve": true, "connect-to": true,
	"aws-sigv4": true,
}

// shortFlags maps a short option letter to its long name.
var shortFlags = map[byte]string{
	'X': "request", 'H': "header", 'd': "data", 'F': "form", 'u': "user",
	'b': "cookie", 'A': "user-agent", 'e': "referer", 'k': "insecure",
	'L': "location", 'x': "proxy", 'm': "max-time", 'G': "get", 'I': "head",
	'E': "cert", 'K': "config", 'c': "cookie-jar", 'o': "output",
	'r': "range", 'w': "write-out", 'z': "time-cond", 'T': "upload-file",
	's': "silent", 'S': "show-error", 'v': "verbose", 'i': "include",
	'f': "fail", 'n': "netrc", 'q': "disable", '4': "ipv4", '6': "ipv6",
	'0': "http1.0", 'O': "remote-name", 'Z': "parallel", '#': "progress-bar",
	// Added for H3 (phase 8 review): every other short flag that takes a
	// value, so its argument is never mistaken for a URL.
	'D': "dump-header", 'P': "ftp-port", 'Q': "quote", 't': "telnet-option",
	'U': "proxy-user", 'Y': "speed-limit", 'y': "speed-time",
	'C': "continue-at", 'N': "no-buffer", 'J': "remote-header-name",
	'g': "globoff", 'B': "use-ascii", 'M': "manual", '1': "tlsv1",
	'2': "sslv2", '3': "sslv3", 'p': "proxy-tunnel", 'a': "append",
	'j': "junk-session-cookies", 'l': "list-only", 'R': "remote-time",
}

// shortValueFlags are the short flags (by long name) that take a value,
// used when splitting a combined short-flag token like "-XPOST".
var shortValueFlags = map[string]bool{
	"request": true, "header": true, "data": true, "form": true,
	"user": true, "cookie": true, "user-agent": true, "referer": true,
	"proxy": true, "max-time": true, "cert": true, "config": true,
	"cookie-jar": true, "output": true, "range": true, "write-out": true,
	"time-cond": true, "upload-file": true,
	// Added for H3 (phase 8 review).
	"dump-header": true, "ftp-port": true, "quote": true,
	"telnet-option": true, "proxy-user": true, "speed-limit": true,
	"speed-time": true, "continue-at": true,
}

// parseArgv parses one curl command's argv (argv[0] == "curl") into p.
// Every argument keeps any $NAME/${NAME} expansion expand.go's tokenizer
// found in it; only the flag syntax itself (--, =, a combined short
// option's letters) is matched against its flattened text, since none of
// that syntax is ever itself templated in a real command.
func parseArgv(argv []templatedString) *parsed {
	p := &parsed{}
	i := 1
	for i < len(argv) {
		arg := argv[i]
		i++
		text := arg.text()
		switch {
		case text == "" || text == "-" || text[0] != '-':
			p.urls = append(p.urls, arg)
		case strings.HasPrefix(text, "--"):
			rest, _ := arg.cutPrefix("--")
			nameTS, value, haveValue := rest.cut("=")
			name := nameTS.text()
			if name == "" {
				continue
			}
			if !haveValue && (longValueFlags[name] || ignoredValueFlags[name]) && i < len(argv) {
				value, haveValue = argv[i], true
				i++
			}
			p.apply(name, value, haveValue)
		default:
			i = p.parseShort(argv, i, arg)
		}
	}
	return p
}

// parseShort applies every flag of a combined short-option token (e.g.
// "-sSL", "-XPOST"): each letter is its own boolean flag, except the
// first one found to take a value, which consumes the rest of the token
// (or, if none is left, the next argv element) as its value and ends the
// token. It returns the index to resume argv scanning from.
func (p *parsed) parseShort(argv []templatedString, next int, arg templatedString) int {
	firstLit := arg.firstLitOrEmpty() // arg.text()[0] == '-', so this is never ""
	for j := 1; j < len(firstLit); j++ {
		long, ok := shortFlags[firstLit[j]]
		if !ok {
			p.warn("-" + string(firstLit[j]))
			continue
		}
		if !shortValueFlags[long] {
			p.apply(long, templatedString{}, false)
			continue
		}
		value := arg.tailFromFirstLit(j + 1)
		haveValue := !value.isEmpty()
		if !haveValue && next < len(argv) {
			value, haveValue = argv[next], true
			next++
		}
		p.apply(long, value, haveValue)
		return next
	}
	return next
}

// apply records one flag occurrence.
func (p *parsed) apply(name string, value templatedString, haveValue bool) {
	switch name {
	case "request":
		if haveValue {
			p.method = value.text()
		}
	case "header":
		if haveValue {
			h, ok := parseHeaderArg(value)
			if !ok {
				// Neither a ':' nor a trailing ';': curl itself sends
				// nothing for this header at all (L1, phase 8 review).
				p.warn("-H " + value.text() + " (no ':' or trailing ';')")
				return
			}
			if h.remove {
				switch {
				case strings.EqualFold(h.name, "Content-Type"):
					// The main case engine/curl.go's own exporter (and
					// this package's own "-d implies a form
					// Content-Type" rule, and --json's implied
					// Content-Type: application/json) can actually
					// reproduce: suppress that implicit header rather
					// than adding it, so curlCommand's own fallback
					// re-emits the identical 'Content-Type:' removal
					// form on export.
					p.suppressContentType = true
				case strings.EqualFold(h.name, "Accept"):
					// The other implied header --json can add (M5/H2,
					// phase 8 review): same reasoning as Content-Type
					// above.
					p.suppressAccept = true
				default:
					// Any other "Name:" removal is otherwise a no-op:
					// this package never adds a header of its own accord
					// besides those two, so warn instead of silently
					// dropping a flag that had no effect (L1, phase 8
					// review).
					p.warn("-H '" + h.name + ":'")
				}
				return
			}
			p.headers = append(p.headers, h)
		}
	case "data", "data-binary", "data-ascii":
		if haveValue {
			p.data = append(p.data, dataArg(value))
		}
	case "data-raw":
		// Unlike -d/--data-binary/--data-ascii, --data-raw never treats
		// a leading '@' as a file reference (curl's own documented
		// exception, so a value that happens to start with '@' can be
		// sent literally without a temp file) -- M3, phase 8 review.
		if haveValue {
			p.data = append(p.data, dataPiece{kind: kindRaw, text: value})
		}
	case "data-urlencode":
		if haveValue {
			piece := urlencodeArg(value)
			if piece.kind == kindUnsupported {
				p.warn("--data-urlencode (file argument)")
			}
			p.data = append(p.data, piece)
		}
	case "json":
		// --json is sugar for --data plus two implied headers (H2,
		// phase 8 review): it shares -d's own file ('@path') and
		// concatenation handling by becoming just another p.data piece,
		// in argv order alongside any -d the same command also has.
		// haveJSON only decides which implied headers buildEntry adds.
		if haveValue {
			p.data = append(p.data, dataArg(value))
			p.haveJSON = true
		}
	case "form":
		if haveValue {
			p.form = append(p.form, p.formArg(value))
		}
	case "form-string":
		// Unlike -F/--form, --form-string never gives '@' or '<' any
		// special meaning: the value is always literal (M5, phase 8
		// review).
		if haveValue {
			nameTS, rest, _ := value.cut("=")
			p.form = append(p.form, formField{name: nameTS.text(), value: rest})
		}
	case "user":
		if haveValue {
			p.user, p.haveUser = value, true
		}
	case "cookie":
		if haveValue {
			p.cookie, p.haveCookie = value, true
		}
	case "user-agent":
		if haveValue {
			p.userAgent, p.haveUA = value, true
		}
	case "referer":
		if haveValue {
			p.referer, p.haveRef = value, true
		}
	case "insecure":
		p.insecure = true
	case "location":
		p.location = true
	case "location-trusted":
		p.locationTrusted = true
	case "compressed":
		p.compressed = true
	case "http1.0":
		p.http10 = true
	case "http1.1":
		p.http11 = true
	case "http2":
		p.http2 = true
	case "http3":
		p.http3 = true
	case "digest":
		p.digest = true
	case "ntlm":
		p.ntlm = true
	case "negotiate":
		p.negotiate = true
	case "ipv4":
		p.ipv4 = true
	case "ipv6":
		p.ipv6 = true
	case "path-as-is":
		p.pathAsIs = true
	case "get":
		p.get = true
	case "head":
		p.head = true
	case "proxy":
		if haveValue {
			p.proxy, p.haveProxy = value, true
		}
	case "connect-timeout":
		if haveValue {
			p.connectTimeout, p.haveConnTimeout = value.text(), true
		}
	case "max-time":
		if haveValue {
			p.maxTime, p.haveMaxTime = value.text(), true
		}
	case "cacert":
		if haveValue {
			p.cacert, p.haveCACert = value, true
		}
	case "cert":
		if haveValue {
			p.cert, p.haveCert = value, true
		}
	case "key":
		if haveValue {
			p.key, p.haveKey = value, true
		}
	case "unix-socket":
		if haveValue {
			p.unixSocket, p.haveUnixSocket = value, true
		}
	case "output":
		if haveValue {
			p.output, p.haveOutput = value, true
		}
	case "pinnedpubkey":
		if haveValue {
			p.pinnedKey, p.havePinnedKey = value, true
		}
	case "max-redirs":
		if haveValue {
			p.maxRedirs, p.haveMaxRedirs = value.text(), true
		}
	case "limit-rate":
		if haveValue {
			p.limitRate, p.haveLimitRate = value.text(), true
		}
	case "resolve":
		if haveValue {
			p.resolve = append(p.resolve, value)
		}
	case "connect-to":
		if haveValue {
			p.connectTo = append(p.connectTo, value)
		}
	case "aws-sigv4":
		if haveValue {
			p.awsSigV4, p.haveAWSSigV4 = value, true
		}
	case "url":
		if haveValue {
			p.urls = append(p.urls, value)
		}
	default:
		if _, ok := booleanFlags[name]; ok {
			return // an informational or unsupported-scheme flag: no warning
		}
		p.warn("--" + name)
	}
}

// parseHeaderArg splits an -H value on its first ':'. Nothing at all
// after the colon ("Name:") is curl's "remove this header" form — it
// suppresses the header entirely, including one curl (or this package's
// own exporter) would otherwise add automatically, rather than sending it
// with an empty value; a value with no ':' but a trailing ';' ("Name;")
// is curl's other special form, sending the header with an empty value.
// The header name itself is always flattened to plain text (see kv).
// ok is false for a value curl itself ignores entirely: neither a ':' nor
// a trailing ';' anywhere in it (L1, phase 8 review) — previously
// returned as a bogus header named after the whole value, with an empty
// value, which curl never sends.
func parseHeaderArg(s templatedString) (h kv, ok bool) {
	if nameTS, value, hasColon := s.cut(":"); hasColon {
		name := strings.TrimSpace(nameTS.text())
		value = value.trimSpace()
		if value.isEmpty() {
			return kv{name: name, remove: true}, true
		}
		return kv{name: name, value: value}, true
	}
	if trimmed, hasSemi := strings.CutSuffix(s.text(), ";"); hasSemi {
		return kv{name: trimmed}, true
	}
	return kv{}, false
}

func dataArg(s templatedString) dataPiece {
	if after, ok := s.cutPrefix("@"); ok {
		return dataPiece{kind: kindFile, text: after}
	}
	return dataPiece{kind: kindRaw, text: s}
}

// urlencodeArg implements --data-urlencode's own sub-syntax: "content" and
// "=content" percent-encode the whole content, "name=content" keeps name
// literal and encodes content; "@file" and "name@file" (curl reads and
// encodes the file itself) are not supported, since this package never
// reads a file a curl command names. curl itself decides which form this
// is by whichever of '=' or '@' appears first in the argument, not by
// whether '@' appears anywhere: "email=a@b.com" is a plain "name=content"
// field whose content happens to contain '@', not a file reference (H1,
// phase 8 review) — cutFirstOf mirrors that by stopping at the first
// match of either byte, leaving anything after it (including a further
// '=' or '@') untouched.
func urlencodeArg(s templatedString) dataPiece {
	if rest, ok := s.cutPrefix("="); ok {
		return dataPiece{kind: kindRaw, text: percentEncodeTemplated(rest)}
	}
	if sep, before, after, ok := s.cutFirstOf("=@"); ok {
		if sep == '@' {
			return dataPiece{kind: kindUnsupported, text: s}
		}
		return dataPiece{kind: kindRaw, text: before.concat("=").add(percentEncodeTemplated(after))}
	}
	return dataPiece{kind: kindRaw, text: percentEncodeTemplated(s)}
}

// formArg parses one -F value: "name=value"; "name=@file" optionally
// followed by any number of ";key=value" attributes (curl allows them in
// any order) — only ";type=..." has a Sonde equivalent (a
// MultipartFile's ContentType), so any other recognized curl attribute
// (filename, headers, encoder) is reported as unsupported, rather than
// silently folded into the file path or content type the way a bare
// ";type=" search used to (M5, phase 8 review); "name=<file" (curl reads
// the file's content as the field's literal text value) is reported as
// unsupported too, since this package never reads a file a curl command
// names. name is always flattened to plain text (see formField).
func (p *parsed) formArg(s templatedString) formField {
	nameTS, rest, _ := s.cut("=")
	name := nameTS.text()
	if content, ok := rest.cutPrefix("<"); ok {
		p.warn("-F " + name + "=<" + content.text() + " (reads a file's content as a literal value)")
		return formField{name: name}
	}
	path, ok := rest.cutPrefix("@")
	if !ok {
		return formField{name: name, value: rest}
	}
	field := formField{name: name, isFile: true}
	for i, attr := range splitTemplated(path, ";") {
		if i == 0 {
			field.file = attr
			continue
		}
		key, value, ok := attr.cut("=")
		if !ok {
			continue
		}
		if strings.TrimSpace(key.text()) == "type" {
			field.contentType = value
			continue
		}
		p.warn("-F " + name + "=@...;" + key.text() + "=... (attribute not supported)")
	}
	return field
}

// percentEncode encodes s the way curl's --data-urlencode does: every
// byte except an unreserved character (RFC 3986: ALPHA / DIGIT / "-" /
// "." / "_" / "~") becomes %XX.
func percentEncode(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0xF])
	}
	return b.String()
}
