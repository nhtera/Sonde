// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package xpath

import (
	"testing"

	"github.com/nhtera/sonde/internal/value"
)

type evalCase struct {
	expr string
	want value.Value
}

func check(t *testing.T, text string, f Format, cases []evalCase) {
	t.Helper()
	doc, err := Parse(text, f)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		got, err := doc.Eval(c.expr)
		if err != nil || !value.Equal(got, c.want) {
			t.Errorf("Eval(%q) = %#v, %v; want %#v", c.expr, got, err, c.want)
		}
	}
}

func TestXML(t *testing.T) {
	xml := `<?xml version="1.0" encoding="utf-8"?>
<food>
  <banana type="fruit" price="1.1"/>
  <apple type="fruit"/>
  <beef type="meat"/>
</food>
`
	check(t, xml, XML, []evalCase{
		{"count(//food/*)", value.Float(3)},
		{"//food/*", value.Nodeset(3)},
		{"count(//*[@type='fruit'])", value.Float(2)},
		{"number(//food/banana/@price)", value.Float(1.1)},
		{"//nothing", value.Nodeset(0)},
		{"boolean(//apple)", value.Bool(true)},
	})
}

func TestEvalErrors(t *testing.T) {
	doc, err := Parse("<a/>", XML)
	if err != nil {
		t.Fatal(err)
	}
	for _, expr := range []string{"^^^", "//", "strong(//head/title)", "//x:y"} {
		if v, err := doc.Eval(expr); err != ErrEval {
			t.Errorf("Eval(%q) = %#v, %v", expr, v, err)
		}
	}
}

func TestInvalidDocument(t *testing.T) {
	for _, in := range []struct {
		text string
		f    Format
	}{{"??", XML}, {"", XML}, {"  \n", HTML}, {"<a>", XML}} {
		if _, err := Parse(in.text, in.f); err == nil {
			t.Errorf("Parse(%q) succeeded", in.text)
		}
	}
}

func TestCafe(t *testing.T) {
	check(t, "<data>café</data>", XML, []evalCase{{"normalize-space(//data)", value.String("café")}})
	check(t, "<data>café</data>", HTML, []evalCase{{"normalize-space(//data)", value.String("café")}})
}

func TestHTML(t *testing.T) {
	html := `<html>
  <head>
    <meta charset="UTF-8"\>
  </head>
  <body>
    <br>
  </body>
</html>`
	check(t, html, HTML, []evalCase{
		{"normalize-space(/html/head/meta/@charset)", value.String("UTF-8")},
	})
	check(t, "<html></html>", HTML, []evalCase{
		{"boolean(count(//a[contains(@href,'xxx')]))", value.Bool(false)},
	})
}

func TestNamespacesWithPrefix(t *testing.T) {
	xml := `<?xml version ="1.0"?>
<a:books xmlns:a="foo:" xmlns:b="bar:">
    <b:book xmlns:c="baz:">
        <b:title>Dune</b:title>
        <c:author>Franck Herbert</c:author>
    </b:book>
</a:books>`
	check(t, xml, XML, []evalCase{
		{"string(//a:books/b:book/b:title)", value.String("Dune")},
		{"string(//a:books/b:book/c:author)", value.String("Franck Herbert")},
		{"string(//*[name()='a:books']/*[name()='b:book']/*[name()='c:author'])", value.String("Franck Herbert")},
		{"string(//*[local-name()='books']/*[local-name()='book']/*[local-name()='author'])", value.String("Franck Herbert")},
	})
}

func TestDefaultNamespaces(t *testing.T) {
	xml := `<svg version="1.1" width="300" height="200" xmlns="http://www.w3.org/2000/svg">
    <rect width="100%" height="100%" fill="red" />
    <circle cx="150" cy="100" r="80" fill="green" />
    <text x="150" y="125" font-size="60" text-anchor="middle" fill="white">SVG</text>
</svg>`
	check(t, xml, XML, []evalCase{
		{"string(//_:svg/_:text)", value.String("SVG")},
		{"string(//*[name()='svg']/*[name()='text'])", value.String("SVG")},
		{"string(//*[local-name()='svg']/*[local-name()='text'])", value.String("SVG")},
	})
}

func TestSoap(t *testing.T) {
	xml := `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"
    xmlns:xsd="http://www.w3.org/2001/XMLSchema"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
    <soap:Body xmlns:ns1="http://www.opentravel.org/OTA/2003/05">
        <ns1:OTA_AirAvailRS
            EchoToken="11868765275150-1300257934"
            TransactionIdentifier="TID$16459590516432752971.demo2144"
            Version="2006.01">
        </ns1:OTA_AirAvailRS>
    </soap:Body>
</soap:Envelope>`
	want := value.String("TID$16459590516432752971.demo2144")
	check(t, xml, XML, []evalCase{
		{"string(//soap:Envelope/soap:Body/ns1:OTA_AirAvailRS/@TransactionIdentifier)", want},
		{"string(//*[name()='soap:Envelope']/*[name()='soap:Body']/*[name()='ns1:OTA_AirAvailRS']/@TransactionIdentifier)", want},
		{"string(//*[local-name()='Envelope']/*[local-name()='Body']/*[local-name()='OTA_AirAvailRS']/@TransactionIdentifier)", want},
	})
}

func TestNamespacesScoping(t *testing.T) {
	xml := `<?xml version="1.0"?>
<!-- initially, the default namespace is "books" -->
<book xmlns='urn:loc.gov:books'
      xmlns:isbn='urn:ISBN:0-395-36341-6'>
    <title>Cheaper by the Dozen</title>
    <isbn:number>1568491379</isbn:number>
    <notes>
      <!-- make HTML the default namespace for some commentary -->
      <p xmlns='http://www.w3.org/1999/xhtml'>
          This is a <i>funny</i> book!
      </p>
    </notes>
</book>
        `
	check(t, xml, XML, []evalCase{
		{"string(//_:book/_:title)", value.String("Cheaper by the Dozen")},
		{"string(//_:book/isbn:number)", value.String("1568491379")},
		{"//*[name()='book']/*[name()='notes']", value.Nodeset(1)},
		{"//_:book/_:notes/*[local-name()='p']", value.Nodeset(1)},
	})
}
