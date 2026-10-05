# Log in
POST {{base_url}}/login
Content-Type: application/json
{"user": "{{user}}", "password": "{{password}}"}
HTTP 200
[Captures]
token: jsonpath "$.token" redact
[Asserts]
jsonpath "$.user.name" == "{{user}}"
