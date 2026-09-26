// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package httpfile

import (
	"fmt"
	"strings"

	"github.com/nhtera/sonde/internal/convert"
	"github.com/nhtera/sonde/internal/syntax"
)

// maxMultipartParts bounds how many parts buildMultipart ever looks at, so a
// pathological body cannot make it do unbounded work.
const maxMultipartParts = 1000

// buildMultipart parses a simple multipart/form-data body into [Multipart]
// fields: every part must carry a "Content-Disposition: form-data;
// name=...` header; a text part's value is its content, and a file part (a
// "filename=" parameter present) takes its local path from its own content,
// a "< path" or "<@ path" reference (Sonde's [Multipart] file field has no
// separate slot for a reported filename distinct from the local path it
// reads, so the Content-Disposition "filename=" parameter itself, matching
// how the REST Client/JetBrains own examples always write it, is not used
// for anything beyond identifying the part as a file). Anything it does not
// recognize reports ok = false, so the caller falls back to a raw body.
func buildMultipart(text, params string) (fields []syntax.MultipartField, warns []convert.Warning, ok bool) {
	boundary := boundaryOf(params)
	if boundary == "" {
		return nil, nil, false
	}
	delim := "--" + boundary
	parts := strings.Split(text, delim)
	if len(parts) < 3 { // preamble + at least one part + closing "--"
		return nil, nil, false
	}
	for _, seg := range parts[1 : len(parts)-1] {
		if len(fields) >= maxMultipartParts {
			return nil, nil, false
		}
		seg = strings.TrimPrefix(strings.TrimPrefix(seg, "\r\n"), "\n")
		headerBlock, bodyBlock, found := strings.Cut(seg, "\n\n")
		if !found {
			return nil, nil, false
		}
		bodyBlock = strings.TrimSuffix(strings.TrimSuffix(bodyBlock, "\n"), "\r")
		name, filename, partCT, hasName := parsePartHeaders(headerBlock)
		if !hasName {
			return nil, nil, false
		}
		keyText, kw := parseRequestText(name)
		warns = append(warns, kw...)
		if filename != "" {
			ref := parseRawBody([]string{bodyBlock})
			if ref.kind == bodyText {
				// No "< path"/"<@ path" reference: the part holds inline
				// content Sonde's [Multipart] file field cannot represent.
				return nil, nil, false
			}
			pathText, nw := parseRequestText(ref.path)
			warns = append(warns, nw...)
			if ref.kind == bodyFileRefSubstituted {
				warns = append(warns, convert.Warning{Kind: convert.WarnUnsupportedBody,
					Message: fmt.Sprintf("multipart field %q: \"<@ %s\" has the source tool substitute its own "+
						"variables inside the file before sending; Sonde's file field sends it as is", name, ref.path)})
			}
			var ctText syntax.Text
			if partCT != "" {
				var cw []convert.Warning
				ctText, cw = parseRequestText(partCT)
				warns = append(warns, cw...)
			}
			fields = append(fields, syntax.MultipartField{Key: keyText, File: &syntax.MultipartFile{Name: pathText, ContentType: ctText}})
			continue
		}
		valText, vw := parseRequestText(strings.TrimSpace(bodyBlock))
		warns = append(warns, vw...)
		fields = append(fields, syntax.MultipartField{Key: keyText, Value: valText})
	}
	if len(fields) == 0 {
		return nil, nil, false
	}
	return fields, warns, true
}

// boundaryOf extracts the boundary token from a Content-Type's parameter
// string ("boundary=X" or `boundary="X"`).
func boundaryOf(params string) string {
	for _, p := range strings.Split(params, ";") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(p), "boundary="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

// parsePartHeaders reads one multipart part's headers: its field name and,
// if it is a file part, its filename and Content-Type.
func parsePartHeaders(block string) (name, filename, contentType string, ok bool) {
	for line := range strings.SplitSeq(block, "\n") {
		line = strings.TrimSuffix(strings.TrimSpace(line), "\r")
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "content-disposition:"):
			name = dispositionParam(line, "name=")
			filename = dispositionParam(line, "filename=")
		case strings.HasPrefix(lower, "content-type:"):
			_, v, found := strings.Cut(line, ":")
			if found {
				contentType = strings.TrimSpace(v)
			}
		}
	}
	return name, filename, contentType, name != ""
}

// dispositionParam extracts a Content-Disposition parameter's value
// ("name=" or "filename="), quoted or not.
func dispositionParam(line, key string) string {
	lower := strings.ToLower(line)
	idx := strings.Index(lower, key)
	if idx < 0 {
		return ""
	}
	rest := line[idx+len(key):]
	if after, ok := strings.CutPrefix(rest, `"`); ok {
		if end := strings.Index(after, `"`); end >= 0 {
			return after[:end]
		}
		return ""
	}
	if end := strings.IndexAny(rest, "; \t"); end >= 0 {
		return rest[:end]
	}
	return rest
}
