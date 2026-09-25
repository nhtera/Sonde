// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"bytes"
	"testing"
)

// benchEntry exercises most constructs: headers, sections, options, a
// JSON body with placeholders, captures, filters and predicates.
const benchEntry = `# Create an order for {{user}}
POST https://api.example.org/v1/orders?source=bench
Authorization: Bearer {{token}}
Content-Type: application/json
[Query]
page: 1
tags: a,b,c
[Options]
retry: 3
delay: 250ms
variable: attempt=1
{
  "customer": "{{user}}",
  "items": [{"sku": "A-1", "qty": 2}, {"sku": "B-2", "qty": 1.5e2}],
  "note": "unicode \u00e9 and escapes \"quoted\"",
  "gift": false,
  "coupon": null
}
HTTP 201
Location: /v1/orders/{{id}}
[Captures]
order_id: jsonpath "$.id"
secret: header "X-Secret" redact
[Asserts]
status == 201
jsonpath "$.items" count == 2
jsonpath "$.items[0].sku" matches /^[A-Z]-\d+$/
header "Content-Type" contains "json"
body split "," nth 0 toInt >= 1
duration < 1000
` + "```" + `
multiline {{value}}
` + "```" + `

`

// benchSource repeats benchEntry up to ~1 MiB.
func benchSource() []byte {
	var buf bytes.Buffer
	for buf.Len() < 1<<20 {
		buf.WriteString(benchEntry)
	}
	return buf.Bytes()
}

func BenchmarkParse(b *testing.B) {
	src := benchSource()
	if _, err := Parse("bench.hurl", src, DialectHurl); err != nil {
		b.Fatalf("bench source does not parse: %v", err)
	}
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Parse("bench.hurl", src, DialectHurl); err != nil {
			b.Fatal(err)
		}
	}
}
