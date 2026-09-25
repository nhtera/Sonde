GET http://localhost:8000/jsonpath/types
HTTP 200
[Asserts]
jsonpath "$[?@.name == 'null'].value" == null
jsonpath "$[?@.name == 'true'].value" == true
jsonpath "$[?@.name == 'false'].value" not exists
jsonpath "$[?@.name == 'fourty_two'].value" == 42
jsonpath "$[?@.name == 'zero'].value" == 0
jsonpath "$[?@.name == 'one_trillion'].value" == 1000000000000
jsonpath "$[?@.name == 'one_hundredth'].value" == 0.01
jsonpath "$[?@.name == 'hello'].value" == "hello"
jsonpath "$[?@.name == 'single_quote'].value" == "'"
jsonpath "$[?@.name == 'double_quote'].value" == "\""
jsonpath "$[?@.name == 'tab'].value" == "\t"
jsonpath "$[?@.name == 'backslash'].value" == "\\"
jsonpath "$[?@.name == 'plane'].value" == "✈"
jsonpath "$[?@.name == 'fire'].value" == "🔥"
jsonpath "$[?@.name == 'array'].value" count == 2
jsonpath "$[?@.name == 'object'].value.a" == 1

jsonpath "$[?@.value == null].name" == "null"
jsonpath "$[?@.value == true].name" == "true"
jsonpath "$[?@.value == false].name" not exists
jsonpath "$[?@.value == 42].name" == "fourty_two"
jsonpath "$[?@.value == -0].name" == "zero"
jsonpath "$[?@.value == 1000000000000].name" == "one_trillion"
jsonpath "$[?@.value == 1e12].name" == "one_trillion"
jsonpath "$[?@.value == 1e+12].name" == "one_trillion"
jsonpath "$[?@.value == 43252003274489856000].name" == "rubikcube_configuration"
jsonpath "$[?@.value == 0.01].name" == "one_hundredth"
jsonpath "$[?@.value == 1e-2].name" == "one_hundredth"
jsonpath "$[?@.value == 'hello'].name" == "hello"
jsonpath "$[?@.value == \"hello\"].name" == "hello"
jsonpath "$[?@.value == \"'\"].name" == "single_quote"
jsonpath "$[?@.value == '\"'].name" == "double_quote"
jsonpath "$[?@.value == '\\t'].name" == "tab"
jsonpath "$[?@.value == '\\\\'].name" == "backslash"
jsonpath "$[?@.value == '🔥'].name" == "fire"
jsonpath "$[?@.value == '\u{1F525}'].name" == "fire"
jsonpath "$[?@.value == '\\uD83D\\uDD25'].name" == "fire"
jsonpath "$[?@.value == '✈'].name" == "plane"
jsonpath "$[?@.value == '\u{2708}'].name" == "plane"
jsonpath "$[?@.value == '\\u2708'].name" == "plane"

[       
  { "name": "null", "value": null },
  { "name": "true", "value": true },
  { "name": "fourty_two", "value": 42 },
  { "name": "zero", "value": 0 },
  { "name": "one_trillion", "value": 1e12 },
  { "name": "rubikcube_configuration", "value": 43252003274489856000},
  { "name": "one_hundredth", "value": 0.01 },
  { "name": "hello", "value": "hello" },
  { "name": "single_quote", "value": "'" },
  { "name": "double_quote", "value": "\"" },
  { "name": "tab", "value": "\t" },
  { "name": "backslash", "value": "\\" },
  { "name": "plane", "value": "✈" },
  { "name": "fire", "value": "🔥" },
  { "name": "array", "value": [1, 2] },
  { "name": "object", "value": { "a": 1 } }
]
