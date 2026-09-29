# Create a user without an email
POST {{base_url}}/users
{"name": "grace"}
HTTP 422
[Asserts]
jsonpath "$.errors[0].field" == "email"
