#!/bin/bash
set -Eeuo pipefail

export HURL_VARIABLE_name=Bob
hurl tests_ok/env_var/env_var.hurl
