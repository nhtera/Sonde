// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import "strings"

// queryParsers lists queries in matching order.
var queryParsers = []parseFunc[*Query]{
	keywordQuery(QueryStatus), keywordQuery(QueryVersion), keywordQuery(QueryURL),
	templateQuery(QueryHeader), cookieQuery, keywordQuery(QueryBody),
	templateQuery(QueryXPath), templateQuery(QueryJSONPath), regexQuery,
	templateQuery(QueryVariable), keywordQuery(QueryDuration), keywordQuery(QueryBytes),
	keywordQuery(QueryRawBytes), keywordQuery(QuerySHA256), keywordQuery(QueryMD5),
	certificateQuery, keywordQuery(QueryIP), sondeStreamQuery, keywordQuery(QueryRedirects),
}

// queryKeywords matches queryParsers; `redirects` stays last so an unknown
// query is reported as in Hurl.
var queryKeywords = []string{"status", "version", "url", "header", "cookie", "body", "xpath",
	"jsonpath", "regex", "variable", "duration", "bytes", "rawbytes", "sha256", "md5",
	"certificate", "ip", "sondeStream", "redirects"}

func query(r *reader) (*Query, *Error) {
	start := r.pos
	q, err := keywordChoice(r, queryKeywords, queryParsers)
	if err != nil {
		return nil, err
	}
	q.Span = Span{start, r.pos}
	return q, nil
}

func keywordQuery(kind QueryKind) parseFunc[*Query] {
	return func(r *reader) (*Query, *Error) {
		if err := tryLiteral(r, kind.String()); err != nil {
			return nil, err
		}
		return &Query{Kind: kind}, nil
	}
}

// templateQuery is `keyword sp "template"`.
func templateQuery(kind QueryKind) parseFunc[*Query] {
	return func(r *reader) (*Query, *Error) {
		if err := tryLiteral(r, kind.String()); err != nil {
			return nil, err
		}
		space0, err := oneOrMoreSpaces(r)
		if err != nil {
			return nil, err
		}
		arg, err := committing(quotedTemplate)(r)
		if err != nil {
			return nil, err
		}
		return &Query{Kind: kind, Space0: space0, Arg: arg}, nil
	}
}

func regexQuery(r *reader) (*Query, *Error) {
	if err := tryLiteral(r, "regex"); err != nil {
		return nil, err
	}
	space0, err := oneOrMoreSpaces(r)
	if err != nil {
		return nil, err
	}
	arg, err := regexValue(r)
	if err != nil {
		return nil, err
	}
	return &Query{Kind: QueryRegex, Space0: space0, Arg: arg}, nil
}

// regexValue is a quoted template or a /regex/ literal.
func regexValue(r *reader) (Node, *Error) {
	v, err := choice(r,
		func(r *reader) (Node, *Error) { return asNode(quotedTemplate(r)) },
		func(r *reader) (Node, *Error) { return asNode(regexLiteral(r)) },
	)
	if err != nil {
		return nil, expecting(err.Pos, false, `" or /`)
	}
	return v, nil
}

func asNode[T Node](v T, err *Error) (Node, *Error) {
	if err != nil {
		return nil, err
	}
	return v, nil
}

func cookieQuery(r *reader) (*Query, *Error) {
	if err := tryLiteral(r, "cookie"); err != nil {
		return nil, err
	}
	space0, err := oneOrMoreSpaces(r)
	if err != nil {
		return nil, err
	}
	open := r.pos
	content, err := quotedOnelineString(r)
	if err != nil {
		return nil, err
	}
	from := Pos{Offset: open.Offset + 1, Line: open.Line, Col: open.Col + 1}
	path, err := cookiePath(r.subReader(from, from.Offset+len(content)))
	if err != nil {
		return nil, err
	}
	path.Source = content
	return &Query{Kind: QueryCookie, Space0: space0, Arg: path}, nil
}

// cookiePath is `name` or `name[Attribute]` inside a cookie query string.
func cookiePath(r *reader) (*CookiePath, *Error) {
	start := r.pos
	nameSrc := r.readWhile(func(c rune) bool { return c != '[' })
	name, err := unquotedTemplate(r.subReader(start, start.Offset+len(nameSrc)))
	if err != nil {
		return nil, err
	}
	attr, _, err := optional(r, cookieAttribute)
	if err != nil {
		return nil, err
	}
	return &CookiePath{Name: name, Attribute: attr}, nil
}

func cookieAttribute(r *reader) (*CookieAttribute, *Error) {
	if err := tryLiteral(r, "["); err != nil {
		return nil, err
	}
	space0, _ := zeroOrMoreSpaces(r)
	start := r.pos
	name := r.readWhile(func(c rune) bool { return isLetter(c) || c == '-' })
	switch strings.ToLower(name) {
	case "value", "expires", "max-age", "domain", "path", "secure", "httponly", "samesite":
	default:
		return nil, errAt(start, false, ErrInvalidCookieAttribute, "")
	}
	space1, _ := zeroOrMoreSpaces(r)
	if err := literal(r, "]"); err != nil {
		return nil, err
	}
	return &CookieAttribute{Space0: space0, Name: name, Space1: space1}, nil
}

var certificateFields = []string{"Subject", "Issuer", "Start-Date", "Expire-Date",
	"Serial-Number", "Subject-Alt-Name", "Value"}

func certificateQuery(r *reader) (*Query, *Error) {
	if err := tryLiteral(r, "certificate"); err != nil {
		return nil, err
	}
	space0, err := oneOrMoreSpaces(r)
	if err != nil {
		return nil, err
	}
	if err := literal(r, `"`); err != nil {
		return nil, err
	}
	for _, f := range certificateFields {
		if r.consume(f + `"`) {
			return &Query{Kind: QueryCertificate, Space0: space0, Arg: &CertificateAttribute{Name: f}}, nil
		}
	}
	return nil, expecting(r.pos, false,
		"Field <Subject>, <Issuer>, <Start-Date>, <Expire-Date>, <Serial-Number>, <Subject-Alt-Name> or <Value>")
}

// filters reads `sp filter` pairs until something else follows.
func filters(r *reader) ([]*FilterItem, *Error) {
	var items []*FilterItem
	for {
		save := r.pos
		space, _ := zeroOrMoreSpaces(r)
		if space.Value == "" {
			return items, nil
		}
		if !r.peekAnyPrefix(filterKeywords) {
			r.pos = save
			return items, nil
		}
		f, err := filter(r)
		if err != nil {
			if err.recoverable {
				r.pos = save
				return items, nil
			}
			return nil, err
		}
		items = append(items, &FilterItem{Space: space, Filter: f})
	}
}

// Filter argument parsers; "strict" ones turn any failure into a fatal one.
const (
	argNone = iota
	argTemplate
	argStrictTemplate
	argRegex
	argInteger
)

// filterArgs lists filters in matching order (longer keywords that share a
// prefix come first) with their argument shapes.
var filterArgs = []struct {
	kind FilterKind
	args [2]int
}{
	{FilterBase64Decode, [2]int{}}, {FilterBase64Encode, [2]int{}},
	{FilterBase64URLSafeDecode, [2]int{}}, {FilterBase64URLSafeEncode, [2]int{}},
	{FilterCharsetDecode, [2]int{argTemplate}}, {FilterCount, [2]int{}},
	{FilterDaysAfterNow, [2]int{}}, {FilterDaysBeforeNow, [2]int{}},
	{FilterDecode, [2]int{argTemplate}}, {FilterFirst, [2]int{}},
	{FilterFormat, [2]int{argTemplate}}, {FilterDateFormat, [2]int{argTemplate}},
	{FilterHTMLUnescape, [2]int{}}, {FilterHTMLEscape, [2]int{}},
	{FilterJSONPath, [2]int{argStrictTemplate}}, {FilterLast, [2]int{}},
	{FilterLocation, [2]int{}}, {FilterNth, [2]int{argInteger}},
	{FilterRegex, [2]int{argRegex}}, {FilterReplaceRegex, [2]int{argRegex, argStrictTemplate}},
	{FilterReplace, [2]int{argStrictTemplate, argStrictTemplate}},
	{FilterSplit, [2]int{argStrictTemplate}}, {FilterToDate, [2]int{argTemplate}},
	{FilterToFloat, [2]int{}}, {FilterToHex, [2]int{}}, {FilterToInt, [2]int{}},
	{FilterToString, [2]int{}}, {FilterURLDecode, [2]int{}}, {FilterURLEncode, [2]int{}},
	{FilterURLQueryParam, [2]int{argStrictTemplate}}, {FilterUTF8Decode, [2]int{}},
	{FilterUTF8Encode, [2]int{}}, {FilterXPath, [2]int{argStrictTemplate}},
}

var filterKeywords = func() []string {
	kws := make([]string, len(filterArgs))
	for i, fa := range filterArgs {
		kws[i] = fa.kind.String()
	}
	return kws
}()

var filterParsers = func() []parseFunc[*Filter] {
	fs := make([]parseFunc[*Filter], len(filterArgs))
	for i, fa := range filterArgs {
		fs[i] = func(r *reader) (*Filter, *Error) {
			if err := tryLiteral(r, fa.kind.String()); err != nil {
				return nil, err
			}
			f := &Filter{Kind: fa.kind}
			if fa.args[0] == argNone {
				return f, nil
			}
			var err *Error
			if f.Space0, err = oneOrMoreSpaces(r); err != nil {
				return nil, err
			}
			if f.Arg, err = filterArg(r, fa.args[0]); err != nil {
				return nil, err
			}
			if fa.args[1] == argNone {
				return f, nil
			}
			if f.Space1, err = oneOrMoreSpaces(r); err != nil {
				return nil, err
			}
			if f.Arg2, err = filterArg(r, fa.args[1]); err != nil {
				return nil, err
			}
			return f, nil
		}
	}
	return fs
}()

func filterArg(r *reader, shape int) (Node, *Error) {
	switch shape {
	case argTemplate:
		return asNode(quotedTemplate(r))
	case argStrictTemplate:
		return asNode(committing(quotedTemplate)(r))
	case argRegex:
		return regexValue(r)
	default: // argInteger
		start := r.pos
		if n, err := integer(r); err == nil {
			return n, nil
		}
		r.pos = start
		ph, err := placeholder(r)
		if err != nil {
			return nil, expecting(err.Pos, false, "integer")
		}
		return ph, nil
	}
}

func filter(r *reader) (*Filter, *Error) {
	start := r.pos
	f, err := keywordChoice(r, filterKeywords, filterParsers)
	if err != nil {
		if err.recoverable {
			return nil, expecting(err.Pos, true, "filter")
		}
		return nil, err
	}
	f.Span = Span{start, r.pos}
	return f, nil
}

func predicate(r *reader) (*Predicate, *Error) {
	p := &Predicate{}
	save := r.pos
	p.Space0 = emptyWhitespace(save)
	if r.consume("not") {
		if sp, err := oneOrMoreSpaces(r); err == nil {
			p.Not, p.Space0 = true, sp
		} else {
			r.pos = save
		}
	}
	start := r.pos
	f, err := keywordChoice(r, predicateKeywords, predicateParsers)
	if err != nil {
		if err.recoverable {
			return nil, errAt(start, false, ErrPredicate, "")
		}
		return nil, err
	}
	f.Span = Span{start, r.pos}
	p.Func = f
	return p, nil
}

// Predicate value constraints.
const (
	valueNone = iota
	valueAny
	valueComparable // number, string or placeholder
	valueBytesLike  // string, hex or base64
	valueMatchable  // string or regex
)

var predicateArgs = []struct {
	kind      PredicateKind
	value     int
	needSpace bool // one or more spaces before the value (else zero or more)
}{
	{PredicateEqual, valueAny, false}, {PredicateNotEqual, valueAny, false},
	{PredicateGreaterOrEqual, valueComparable, false}, {PredicateGreater, valueComparable, false},
	{PredicateLessOrEqual, valueComparable, false}, {PredicateLess, valueComparable, false},
	{PredicateStartWith, valueBytesLike, true}, {PredicateEndWith, valueBytesLike, true},
	{PredicateContain, valueAny, true}, {PredicateInclude, valueAny, true},
	{PredicateMatch, valueMatchable, true},
	{PredicateIsInteger, valueNone, false}, {PredicateIsFloat, valueNone, false},
	{PredicateIsBoolean, valueNone, false}, {PredicateIsString, valueNone, false},
	{PredicateIsCollection, valueNone, false}, {PredicateIsList, valueNone, false},
	{PredicateIsObject, valueNone, false}, {PredicateIsDate, valueNone, false},
	{PredicateIsIsoDate, valueNone, false}, {PredicateExist, valueNone, false},
	{PredicateIsEmpty, valueNone, false}, {PredicateIsNumber, valueNone, false},
	{PredicateIsIPv4, valueNone, false}, {PredicateIsIPv6, valueNone, false},
	{PredicateIsUUID, valueNone, false},
}

var predicateKeywords = func() []string {
	kws := make([]string, len(predicateArgs))
	for i, pa := range predicateArgs {
		kws[i] = pa.kind.String()
	}
	return kws
}()

var predicateParsers = func() []parseFunc[*PredicateFunc] {
	fs := make([]parseFunc[*PredicateFunc], len(predicateArgs))
	for i, pa := range predicateArgs {
		fs[i] = func(r *reader) (*PredicateFunc, *Error) {
			if err := tryLiteral(r, pa.kind.String()); err != nil {
				return nil, err
			}
			f := &PredicateFunc{Kind: pa.kind}
			if pa.value == valueNone {
				return f, nil
			}
			var err *Error
			if pa.needSpace {
				f.Space0, err = oneOrMoreSpaces(r)
			} else {
				f.Space0, err = zeroOrMoreSpaces(r)
			}
			if err != nil {
				return nil, err
			}
			start := r.pos
			if f.Value, err = predicateValue(r); err != nil {
				return nil, err
			}
			if !valueAllowed(pa.value, f.Value) {
				return nil, errAt(start, false, ErrPredicateValue, "")
			}
			return f, nil
		}
	}
	return fs
}()

func valueAllowed(constraint int, v PredicateValue) bool {
	switch v.(type) {
	case *Number, *Placeholder:
		return constraint == valueAny || constraint == valueComparable
	case *Template:
		return true
	case *Hex, *Base64:
		return constraint == valueAny || constraint == valueBytesLike
	case *Regex:
		return constraint == valueAny || constraint == valueMatchable
	}
	return constraint == valueAny
}

func predicateValue(r *reader) (PredicateValue, *Error) {
	v, err := prefixChoice(r, []alt[PredicateValue]{
		{[]string{"null"}, func(r *reader) (PredicateValue, *Error) { return asPredicateValue(null(r)) }},
		{[]string{"true", "false"}, func(r *reader) (PredicateValue, *Error) { return asPredicateValue(boolean(r)) }},
		{numberPrefixes, func(r *reader) (PredicateValue, *Error) { return asPredicateValue(number(r)) }},
		{[]string{"file"}, func(r *reader) (PredicateValue, *Error) { return asPredicateValue(fileRef(r)) }},
		{[]string{"hex"}, func(r *reader) (PredicateValue, *Error) { return asPredicateValue(hexBytes(r)) }},
		{[]string{"base64"}, func(r *reader) (PredicateValue, *Error) { return asPredicateValue(base64Bytes(r)) }},
		{[]string{"{{"}, func(r *reader) (PredicateValue, *Error) { return asPredicateValue(placeholder(r)) }},
		{[]string{`"`}, func(r *reader) (PredicateValue, *Error) { return asPredicateValue(quotedTemplate(r)) }},
		{[]string{"```"}, func(r *reader) (PredicateValue, *Error) { return asPredicateValue(multilineString(r)) }},
		{[]string{"`"}, func(r *reader) (PredicateValue, *Error) { return asPredicateValue(backtickTemplate(r)) }},
		{[]string{"/"}, func(r *reader) (PredicateValue, *Error) { return asPredicateValue(regexLiteral(r)) }},
	})
	if err != nil {
		if err.recoverable {
			return nil, errAt(err.Pos, false, ErrPredicateValue, "")
		}
		return nil, err
	}
	return v, nil
}

func asPredicateValue[T PredicateValue](v T, err *Error) (PredicateValue, *Error) {
	if err != nil {
		return nil, err
	}
	return v, nil
}
