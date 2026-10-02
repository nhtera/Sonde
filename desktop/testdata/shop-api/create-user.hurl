POST {{base_url}}/users
Content-Type: application/json
{
  "name": "{{name}}",
  "email": "{{email}}"
}
HTTP 201
[Captures]
user_id: jsonpath "$.id"

GET {{base_url}}/users/{{user_id}}
HTTP 200
[Asserts]
jsonpath "$.email" == "{{email}}"
