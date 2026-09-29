# Log in as each row's user
POST {{base_url}}/login
{"user": "{{user}}", "password": "{{pass}}"}
HTTP *
[Captures]
session_token: jsonpath "$.token" redact
