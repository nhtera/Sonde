#!/bin/bash
set -Eeuo pipefail

export HURL_USER=bob@email.com:secret
hurl tests_ok/basic_authentication/basic_authentication.hurl
