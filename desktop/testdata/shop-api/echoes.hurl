# A binary body that echoes a secret
GET {{base_url}}/binary
X-Token: {{password}}
HTTP 200

# A gzip body that echoes a secret
GET {{base_url}}/gzip
X-Token: {{password}}
HTTP 200
[Asserts]
jsonpath "$.echo" exists
