Set-StrictMode -Version latest
$ErrorActionPreference = 'Stop'

hurl --no-pretty --no-output tests_pty/output/output_option_stdout.hurl
