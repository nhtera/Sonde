Set-StrictMode -Version latest
$ErrorActionPreference = 'Stop'

New-Item -Path build\tmp -Force -ItemType Directory
hurl --file-root build tests_failed/output/output_directory_option.hurl
