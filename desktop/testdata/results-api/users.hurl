# Log in
POST {{api}}/login
{"user": "{{user}}", "password": "{{password}}"}
HTTP 200
[Captures]
token: jsonpath "$.token" redact
user_id: jsonpath "$.user.id"

# Me
GET {{api}}/users/{{user_id}}
Authorization: Bearer {{token}}
HTTP 200
[Asserts]
jsonpath "$.name" == "ada"
jsonpath "$.name" == "grace"
