GET {{base}}/html
HTTP 200
[Asserts]
xpath "string(//title)" == "Café & Co"
xpath "normalize-space(//p[2])" == "twothree"
xpath "count(//p)" == 2
xpath "count(//p)" == 2.0
xpath "//li" count == 2
xpath "//p" == 2
xpath "//nothing" exists
xpath "//nothing" count == 0
xpath "boolean(//ul)" == true
xpath "string(//p[@class='a'])" == "one"
xpath "number(//li[2])" == 2
xpath "count(//head)" == 1
xpath "//[" == 1
body contains "Café"
body xpath "string(//title)" == "Café & Co"
GET {{base}}/xml
HTTP 200
[Asserts]
xpath "string(//_:book/_:title)" == "Dune"
xpath "string(//isbn:number)" == "42"
xpath "number(//_:price)" == 9.5
xpath "string(//_:price/@cur)" == "EUR"
xpath "count(//title)" == 0
xpath "string(//*[local-name()='title'])" == "Dune"
GET {{base}}/badjson
HTTP 200
[Asserts]
jsonpath "$.a" == 1
body jsonpath "$.a" == 1
body == "{\"a\": 1,}"
xpath "/" exists
