# Log in
POST {{base_url}}/login
Content-Type: application/json
{"user": "{{user}}", "password": "{{password}}"}
HTTP 200
[Captures]
token: jsonpath "$.token" redact

# New cart
POST {{base_url}}/carts
Authorization: Bearer {{token}}
HTTP 201
[Captures]
cart_id: jsonpath "$.id"

# Add an item
POST {{base_url}}/carts/{{cart_id}}/items
Authorization: Bearer {{token}}
Content-Type: application/json
{"sku": "TEA-EARL-250", "quantity": 2}
HTTP 200
[Asserts]
jsonpath "$.items" == 1

# Check out
POST {{base_url}}/carts/{{cart_id}}/checkout
Authorization: Bearer {{token}}
HTTP 200
[Captures]
order_id: jsonpath "$.order"

# The order is paid
GET {{base_url}}/orders/{{order_id}}
Authorization: Bearer {{token}}
HTTP 200
[Asserts]
jsonpath "$.status" == "paid"
