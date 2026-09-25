Set-StrictMode -Version latest
$ErrorActionPreference = 'Stop'

hurl --insecure tests_ssl/insecure.hurl
