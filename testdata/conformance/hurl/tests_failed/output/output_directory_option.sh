#!/bin/bash
set -Eeuo pipefail

mkdir -p build/tmp
hurl --file-root build tests_failed/output/output_directory_option.hurl
