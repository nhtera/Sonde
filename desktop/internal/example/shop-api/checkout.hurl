# New cart
POST {{base_url}}/carts
HTTP 201
[Captures]
cart_id: jsonpath "$.id"

# Add an item
POST {{base_url}}/carts/{{cart_id}}/items
Content-Type: application/json
{"sku": "{{sku}}", "quantity": 2}
HTTP 200
[Asserts]
jsonpath "$.items" count == 1
jsonpath "$.subtotal" == 25.8

# Order it
POST {{base_url}}/orders
Content-Type: application/json
{"cart_id": "{{cart_id}}"}
HTTP 201
[Asserts]
jsonpath "$.status" == "pending"
