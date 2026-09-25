GET {{base}}/json
HTTP 200
[Asserts]
jsonpath "$.str" == "café"
jsonpath "$.str" count == 4
jsonpath "$.str" isEmpty
jsonpath "$.empty" isEmpty
jsonpath "$.str" contains "af"
jsonpath "$.str" startsWith "ca"
jsonpath "$.str" endsWith "fé"
jsonpath "$.str" matches /^\w+$/
jsonpath "$.unicode" matches /^\d+$/
jsonpath "$.words" matches "^\\w+ \\w+$"
jsonpath "$.words" split " " count == 2
jsonpath "$.csv" split "," == 3
jsonpath "$.csv" split "," nth 2 == ""
jsonpath "$.csv" split "," nth -1 == "c"
jsonpath "$.csv" split "," nth 9 == "c"
jsonpath "$.csv" split "" count == 9
jsonpath "$.csv" replace "," ";" == "a;b;;c"
jsonpath "$.csv" replace "" "-" == "-a-,-b-,-,-c-"
jsonpath "$.csv" replaceRegex /(\w)/ "<$1>" == "<a>,<b>,,<c>"
jsonpath "$.words" regex /(\w+) / == "héllo"
jsonpath "$.words" regex /(\d+)/ == "x"
jsonpath "$.html" htmlUnescape == "<p class=\"x\">café &amp '\u{27}€"
jsonpath "$.html" htmlUnescape htmlEscape == "x"
jsonpath "$.url" urlQueryParam "c" == "hello world x"
jsonpath "$.url" urlQueryParam "z" == "x"
jsonpath "$.url" urlDecode == "https://x.org/a?b=1&c=hello world+x"
jsonpath "$.words" urlEncode == "h%C3%A9llo%20w%C3%B6rld"
jsonpath "$.b64" base64Decode == hex,48656c6c6f;
jsonpath "$.b64" base64Decode utf8Decode == "Hello"
jsonpath "$.b64url" base64UrlSafeDecode toHex == "fbff"
jsonpath "$.b64" base64UrlSafeDecode toHex == "x"
jsonpath "$.str" utf8Encode base64Encode == "Y2Fmw6k="
jsonpath "$.str" utf8Encode base64UrlSafeEncode == "Y2Fmw6k"
jsonpath "$.str" utf8Encode count == 5
jsonpath "$.str" utf8Encode toHex == "636166c3a9"
jsonpath "$.str" base64Encode == "x"
jsonpath "$.str" toString == "café"
jsonpath "$.list" toString == "x"
jsonpath "$.bool" toString == "false"
jsonpath "$.nulls" toString == "null"
jsonpath "$.nulls" == null
jsonpath "$.nulls" isEmpty
jsonpath "$.obj" isEmpty
jsonpath "$.obj" count == 3
jsonpath "$.obj" isObject
jsonpath "$.list" isList
jsonpath "$.list" isCollection
jsonpath "$.list" contains "three"
jsonpath "$.list" contains 2
jsonpath "$.list" includes null
jsonpath "$.list" contains "four"
jsonpath "$.str" includes "c"
jsonpath "$.date" isIsoDate
jsonpath "$.str" isIsoDate
jsonpath "$.int" isIsoDate
jsonpath "$.str" isUuid
jsonpath "$.str" isIpv4
jsonpath "$.list" first == 1
jsonpath "$.list" last isObject
jsonpath "$.empty" first == 1
