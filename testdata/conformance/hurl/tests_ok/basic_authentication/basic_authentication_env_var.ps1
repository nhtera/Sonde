Set-StrictMode -Version latest
$ErrorActionPreference = 'Stop'

$env:HURL_USER = 'bob@email.com:secret'
hurl tests_ok/basic_authentication/basic_authentication.hurl
