# List the products
GET {{base_url}}/products
HTTP 200
[Asserts]
jsonpath "$.items" count == 2
jsonpath "$.items[0].sku" == "{{sku}}"

# One product
GET {{base_url}}/products/{{sku}}
HTTP 200
[Asserts]
jsonpath "$.name" == "Earl Grey, 250 g"
jsonpath "$.stock" > 0
