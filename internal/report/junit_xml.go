// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// xmlAttr is one XML attribute (name, value).
type xmlAttr struct {
	Name  string
	Value string
}

// xmlNode is either an element (Name != "") or a text node (Name == "",
// Text holds the content). A minimal in-memory XML tree, just expressive
// enough for a JUnit report: elements, attributes and text.
type xmlNode struct {
	Name     string
	Attrs    []xmlAttr
	Children []*xmlNode
	Text     string
}

// newXMLElement returns an element node named name with attrs, in order.
func newXMLElement(name string, attrs ...xmlAttr) *xmlNode {
	return &xmlNode{Name: name, Attrs: attrs}
}

// addChild appends child to n's children and returns n.
func (n *xmlNode) addChild(child *xmlNode) *xmlNode {
	n.Children = append(n.Children, child)
	return n
}

// addText appends a text child holding text and returns n.
func (n *xmlNode) addText(text string) *xmlNode {
	return n.addChild(&xmlNode{Text: text})
}

// writeXMLDocument writes the "<?xml ...?>" declaration followed by root,
// matching the reference implementation's xml-rs-based writer: no
// indentation, elements with no children self-close ("<a />"), text is
// escaped for "&", "<" and ">" only, attribute values additionally for
// '"'.
func writeXMLDocument(w io.Writer, root *xmlNode) error {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	root.write(&buf)
	_, err := w.Write(buf.Bytes())
	return err
}

func (n *xmlNode) write(buf *bytes.Buffer) {
	if n.Name == "" {
		writeXMLText(buf, n.Text)
		return
	}
	buf.WriteByte('<')
	buf.WriteString(n.Name)
	for _, a := range n.Attrs {
		buf.WriteByte(' ')
		buf.WriteString(a.Name)
		buf.WriteString(`="`)
		writeXMLAttrValue(buf, a.Value)
		buf.WriteByte('"')
	}
	if len(n.Children) == 0 {
		buf.WriteString(" />")
		return
	}
	buf.WriteByte('>')
	for _, c := range n.Children {
		c.write(buf)
	}
	buf.WriteString("</")
	buf.WriteString(n.Name)
	buf.WriteByte('>')
}

func writeXMLText(buf *bytes.Buffer, s string) {
	for _, r := range s {
		r = validXMLRune(r)
		switch r {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		default:
			buf.WriteRune(r)
		}
	}
}

func writeXMLAttrValue(buf *bytes.Buffer, s string) {
	for _, r := range s {
		r = validXMLRune(r)
		switch r {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		case '"':
			buf.WriteString("&quot;")
		default:
			buf.WriteRune(r)
		}
	}
}

// validXMLRune returns r unchanged when it is legal in an XML 1.0 document
// (https://www.w3.org/TR/xml/#charsets), or the replacement character
// (U+FFFD) otherwise. A source string ranged over with "for range" already
// turns invalid UTF-8 into U+FFFD on its own; this additionally catches
// the control characters (other than tab/LF/CR), surrogate code points and
// U+FFFE/U+FFFF that are valid UTF-8 but still illegal in XML — a runtime
// error message can carry any of these when it quotes a response body.
// Left unreplaced, they would make the file invalid XML, and the next
// cumulative WriteJUnit would fail to parse it back.
func validXMLRune(r rune) rune {
	switch {
	case r == 0x9 || r == 0xA || r == 0xD,
		r >= 0x20 && r <= 0xD7FF,
		r >= 0xE000 && r <= 0xFFFD,
		r >= 0x10000 && r <= 0x10FFFF:
		return r
	default:
		return utf8.RuneError
	}
}

// parseXMLDocument parses data (a whole XML document, as produced by
// writeXMLDocument) into its root element. Whitespace-only text between
// elements is dropped, matching the reference reader (an xml-rs
// `Whitespace` event, as opposed to `Characters`, is ignored); any other
// text is kept verbatim.
func parseXMLDocument(data []byte) (*xmlNode, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var root *xmlNode
	var stack []*xmlNode
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			el := &xmlNode{Name: t.Name.Local}
			for _, a := range t.Attr {
				el.Attrs = append(el.Attrs, xmlAttr{Name: a.Name.Local, Value: a.Value})
			}
			if len(stack) > 0 {
				stack[len(stack)-1].addChild(el)
			} else if root == nil {
				root = el
			}
			stack = append(stack, el)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) == 0 || strings.TrimSpace(string(t)) == "" {
				continue
			}
			stack[len(stack)-1].addText(string(t))
		}
	}
	if root == nil {
		return nil, errors.New("report: empty XML document")
	}
	return root, nil
}
