# A binary body that echoes a secret
GET {{api}}/binary
X-Token: {{password}}
HTTP 200

# A gzip body that echoes a secret
GET {{api}}/gzip
X-Token: {{password}}
HTTP 200
[Asserts]
jsonpath "$.echo" exists
