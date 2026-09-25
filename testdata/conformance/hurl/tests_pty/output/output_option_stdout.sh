#!/bin/bash
set -Eeuo pipefail

hurl --no-output tests_pty/output/output_option_stdout.hurl
