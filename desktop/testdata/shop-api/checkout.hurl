# Log in
POST {{base_url}}/login
{"user": "{{user}}", "password": "{{password}}"}
HTTP 200
[Captures]
token: jsonpath "$.token" redact
user_id: jsonpath "$.user.id"

# Me
GET {{base_url}}/users/{{user_id}}
Authorization: Bearer {{token}}
HTTP 200

# New cart
POST {{base_url}}/carts
Authorization: Bearer {{token}}
HTTP 201
[Captures]
cart_id: jsonpath "$.id"

# Add an item
POST {{base_url}}/carts/{{cart_id}}/items
Authorization: Bearer {{token}}
{"sku": "tea"}
HTTP 200

# Checkout
POST {{base_url}}/carts/{{cart_id}}/checkout
Authorization: Bearer {{token}}
HTTP 200
[Asserts]
jsonpath "$.status" == "paid"
