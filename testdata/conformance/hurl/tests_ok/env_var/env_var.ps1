Set-StrictMode -Version latest
$ErrorActionPreference = 'Stop'

$env:HURL_VARIABLE_name = 'Bob'
hurl tests_ok/env_var/env_var.hurl
