# 0003: v1.0 compatibility result

Status: Accepted. Date: 2026-09-27.

## Context

The v1.0 plan set a compatibility gate on the blocking lane of the
conformance suite (the vendored Hurl 8.0.1 integration tests that need only
the local test server, see `docs/conformance.md`): a semantic pass rate of
at least 90%. If the time box ended between 85% and 90%, v1.0 would ship
anyway, with the gap recorded here. This record states where v1.0 landed
and which gaps are permanent rather than pending.

## Result

The target was met, so the fallback was not used.

| Lane | Scripts | Result |
|---|---|---|
| blocking | 298 (1 skipped by the reference runner itself) | 289 of 297 pass: **97.3% semantic**, 93.9% full oracle |
| extended | 20 (4 skipped) | 14 of 16 pass: 87.5% semantic (report-only) |
| timing | 22 | 22 pass semantically, 77.3% full oracle (quarantined) |
| network | 17 | not run by default (needs internet hosts) |

"Semantic" compares the exit code and stdout with the reference; "full
oracle" compares stderr too (`docs/stability.md`).

## Permanent gaps

These 8 blocking-lane scripts fail by design, and `docs/compat.md` lists
each one:

- **HTTP/1.0** (`--http1.0`, `HURL_HTTP10`; 2 scripts). Go's `net/http`
  always writes an HTTP/1.1 request line, whatever `Request.Proto` says.
  Sending HTTP/1.0 would need a hand-written client for one rarely used
  option.
- **Digest and NTLM authentication** (4 scripts) and **AWS SigV4 signing**
  (1 script). Not implemented: low weight in the suite and in real use.
  They can be added in a 1.y release without breaking anything, since the
  options already exist and fail with a clear `unsupported` error.
- **`--version` output** (1 script). The reference prints its own program
  name and libcurl features; Sonde prints its own version, commit, build
  date and Go version. Matching it would be wrong, not compatible.

Among the differences that fail the full oracle but not the semantic
check, one is also permanent:

- **Response header order.** Go's `net/http` does not keep the wire order
  of headers with different names, so Sonde prints them grouped by name
  with names sorted (the values of one name keep their order).

## Consequences

- v1.0 ships with the compatibility claims in `docs/compat.md` as they
  stand. The manifest (`internal/conformance/manifest.yaml`) gates every
  change: a blocking-lane script expected to pass that fails is a
  regression.
- Adding Digest, NTLM or AWS SigV4 later turns their scripts to `pass`
  and needs no decision. Supporting HTTP/1.0 or wire-order headers would
  mean replacing `net/http`'s client and needs a new record.
