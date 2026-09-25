GET {{base}}/neg
HTTP 200
[Asserts]
jsonpath "$[?@ == -1.5]" == -1.5
jsonpath "$.list[?@ > -1.5]" count == 3
jsonpath "$.list[?@ < -0.5]" == -1
jsonpath "$.list[?@ == -0.25]" == -0.25
