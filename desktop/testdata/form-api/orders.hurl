# Orders, for the Form view's tests: comments must survive every edit.

# Get an order
GET {{base_url}}/orders/o-1
# who asks
Authorization: Bearer {{token}}
Accept-Language: en-GB
# X-Debug: 1
[Query]
expand: items
HTTP 200
[Asserts]
# the id first
jsonpath "$.id" == "o-1"
jsonpath "$.status" == "paid"

# Create a user
POST {{base_url}}/users
Content-Type: application/json
{"name": "grace"}
HTTP 422
