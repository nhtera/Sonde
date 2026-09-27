# Security Policy

## Reporting a vulnerability

Please **do not** open a public issue. Report vulnerabilities privately via
GitHub: **Security → Report a vulnerability** on
[nhtera/sonde](https://github.com/nhtera/sonde/security/advisories/new).

Include the Sonde version (`sonde version`), OS, a minimal reproduction
(request file and command line) and the impact you observed.

You will get an acknowledgement within 7 days. Fixes are released as patch
versions and disclosed through a GitHub security advisory.

## Supported versions

Before v1.0.0 only the latest release receives security fixes.

From v1.0.0: only the latest minor release line gets security fixes,
shipped as a patch release on that line (for example, a fix for v1.2.x
ships as v1.2.<next>, not backported to v1.1.x). Upgrade to the latest
minor to stay covered; see [docs/stability.md](docs/stability.md) for what
v1's compatibility promise does and does not cover.

## Advisory dry run (maintainers)

A checklist for drafting and shipping a fix once a report is confirmed:

1. Open a private GitHub Security Advisory on this repository (**Security
   → Advisories → New draft advisory**), not a public issue or PR. Fill in
   affected versions, a CVSS vector, and a plain-language impact summary.
2. Reproduce the issue on a private branch; add a regression test that
   fails before the fix and passes after (mirrors this repo's normal
   testing bar — see CONTRIBUTING.md).
3. Request a CVE identifier from the advisory's own "Request CVE ID"
   button once the draft is ready; GitHub mints one via its CNA.
4. Prepare the patch release (next patch on the latest minor line) in a
   temporary private fork tied to the advisory, so CI runs against the fix
   without exposing it publicly ahead of release.
5. Publish the advisory and tag the release together: the advisory should
   go public within minutes of the fixed artifacts landing (GitHub
   Releases, `go install`, Homebrew tap, Scoop bucket), not before.
6. Credit the reporter (with their permission) in the advisory and the
   release notes.
