# Copyright 2026 The Sonde Authors
# SPDX-License-Identifier: Apache-2.0
#
# Runs one tests_pty script through the upstream PseudoTerm (vendored
# integration/term.py: stdout and stderr each on its own raw pty, 24 rows x
# 100 columns, as integration/hurl/integration.py runs tests_pty; the script
# runs under bash rather than through its #!/bin/bash shebang)
# and writes what each pty received, plus the exit code, to files. The Go
# harness scores them with its own oracle port, so this file only captures.
#
# Usage: pty-capture.py INTEGRATION_DIR SCRIPT STDOUT_FILE STDERR_FILE EXIT_FILE
import sys


def main() -> None:
    integration_dir, script, out_path, err_path, exit_path = sys.argv[1:6]
    sys.dont_write_bytecode = True  # never write __pycache__ into the vendored tree
    sys.path.insert(0, integration_dir)
    from term import PseudoTerm

    result = PseudoTerm(row=24, col=100).run(cmd=["bash", script])
    with open(out_path, "wb") as f:
        f.write(result.stdout)
    with open(err_path, "wb") as f:
        f.write(result.stderr)
    with open(exit_path, "w", encoding="utf-8") as f:
        f.write(str(result.returncode))


if __name__ == "__main__":
    main()
