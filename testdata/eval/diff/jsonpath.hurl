GET {{base}}/json
HTTP 200
[Asserts]
jsonpath "$.obj.*" count == 3
jsonpath "$.obj.*" nth 0 == 1
jsonpath "$.obj.*" nth 1 == 2
jsonpath "$..y" == 25
jsonpath "$..*" count == 7
jsonpath "$.books[*].title" == "x"
jsonpath "$.books[?(@.price < 10)].title" nth 1 == "Ubik"
jsonpath "$.books[?@.price > 10].title" == "Emma"
jsonpath "$.books[?length(@.tags) == 0].title" == "Emma"
jsonpath "$.books[-1].title" == "Ubik"
jsonpath "$.books[0:2]" count == 2
jsonpath "$.books[0].tags" == "sf"
jsonpath "$.books[0].tags" count == 1
jsonpath "$.books[1].tags" count == 0
jsonpath "$.books[5]" not exists
jsonpath "$.obj" == "x"
jsonpath "$.list[5]" isObject
jsonpath "$.list[?@ == 2]" == 2.0
jsonpath "$['obj']['c']['z']" == 26
jsonpath "$.obj[" == 1
jsonpath "$.books[*].price" jsonpath "$[0]" == 1
jsonpath "$.str" jsonpath "$" == 1
jsonpath "$.books[?@.price > -1.5].title" count == 3
jsonpath "$.list[?@ > -0.75]" count == 3
jsonpath "$.list[?@ < -0.75]" count == 0
jsonpath "$['😀']" exists
jsonpath "$['😀']" exists
jsonpath "$.books[?@.price == 8.99].title" == "Dune"
jsonpath "$.books[?@.price == 899e-2].title" == "Dune"
