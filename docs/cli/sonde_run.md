<!-- Generated from the command tree by `make docs`. Do not edit. -->

# sonde run

Run request files (same as `sonde FILE...`).

```
sonde run [options] FILE...
```

## Flags

```
      --aws-sigv4 string             signs the request with AWS Signature Version 4 (not supported by sonde)
      --cacert string                CA certificate bundle used to verify the server (PEM)
  -E, --cert string                  client certificate file, optionally with :PASSWORD
      --compressed                   requests a compressed response and decodes it
      --config string                uses this sonde.yaml for every input file, skipping discovery
      --connect-timeout string       maximum time allowed to establish the connection
      --connect-to stringArray       redirects connections for HOST1:PORT1 to HOST2:PORT2
      --continue-on-error            keeps running remaining files after a failure
  -b, --cookie string                reads cookies from a Netscape-format FILE
  -c, --cookie-jar string            writes cookies to FILE after the run
      --curl string                  exports each request as a list of curl commands
      --data string                  runs each file once per row of a CSV or JSON data file
      --data-secret strings          marks data file columns as secrets (comma-separated, repeatable)
      --delay string                 sleep before each request
      --digest                       uses HTTP Digest authentication (not supported by sonde)
      --env string                   selects a sonde.yaml environment by name
      --error-format string          controls how error messages are rendered (short or long)
      --fail-with-body               writes the response body of a failed entry before its errors
      --file-root string             sets the root directory used to resolve file paths
      --from-entry int               starts execution at the given entry number
      --glob stringArray             adds input files matching the given glob pattern
  -H, --header stringArray           adds a custom header to every request
  -0, --http1.0                      forces HTTP/1.0
      --http1.1                      forces HTTP/1.1
      --http2                        forces HTTP/2
      --http2-prior-knowledge        uses HTTP/2 without an HTTP/1.1 upgrade
      --http3                        forces HTTP/3
  -i, --include                      includes the response headers in the output
  -k, --insecure                     skips TLS certificate verification
  -4, --ipv4                         resolves hostnames to IPv4 addresses only
  -6, --ipv6                         resolves hostnames to IPv6 addresses only
      --jobs int                     maximum number of parallel jobs, 1 disables parallelism
      --json                         outputs each file's result as JSON
      --key string                   private key file matching --cert
      --limit-rate string            caps the transfer rate in bytes per second
  -L, --location                     follows HTTP redirects
      --location-trusted             follows redirects and forwards credentials to every host
      --max-filesize string          caps the size of a downloaded file
      --max-redirs string            maximum number of redirects to follow, -1 for unlimited
  -m, --max-time string              maximum time allowed for the whole transfer
      --negotiate                    uses SPNEGO authentication (not supported by sonde)
  -n, --netrc                        reads credentials from ~/.netrc, failing if absent
      --netrc-file string            reads credentials from the given netrc-format file
      --netrc-optional               reads credentials from ~/.netrc if present, else the URL
      --no-assert                    ignores asserts defined in the file
      --no-cookie-store              disables the cookie store between requests
      --no-header stringArray        removes a header sent to the server, a default one included
      --no-jsonpath-coercion         keeps jsonpath results a list of matches
      --no-output                    suppresses the default last-response-body output
      --no-pretty                    disables pretty-printing of response output
      --no-proxy string              lists hosts that bypass the proxy
      --ntlm                         uses NTLM authentication (not supported by sonde)
      --openapi string               validates every response against an OpenAPI spec (file, or URL with --openapi-allow-remote)
      --openapi-allow-remote         allows a remote spec and remote $ref targets
      --openapi-server string        base URL replacing the spec's servers when matching requests to operations
      --openapi-strict               fails a request that no operation of the spec matches
  -o, --output string                writes to FILE instead of stdout
      --parallel                     runs files in parallel (default in test mode)
      --path-as-is                   sends the URL path without normalizing /../ or /./
      --pinnedpubkey string          verifies the server's public key against pinned hashes
      --pretty                       pretty-prints JSON response output
      --progress-bar                 shows a progress bar in test mode
  -x, --proxy string                 routes the request through the given proxy
      --proxy-header stringArray     adds a header sent to the proxy only
      --repeat string                repeats the input file sequence N times, -1 for infinite
      --report-html string           writes an HTML report to DIR
      --report-json string           writes a JSON report to DIR
      --report-junit string          writes a JUnit XML report to FILE
      --report-tap string            writes a TAP report to FILE
      --resolve stringArray          provides a custom address for a HOST:PORT pair
      --retry string                 maximum retries on entry error, -1 for unlimited
      --retry-interval string        delay between retries
      --secret stringArray           defines a variable whose value is treated as a secret
      --secrets-file stringArray     defines secrets from a file
      --ssl-no-revoke                disables certificate revocation checks (not supported by sonde)
      --test                         activates test mode (parallel execution, test-style output)
      --to-entry int                 stops execution at the given entry number
      --unix-socket string           connects through a Unix domain socket instead of the network
  -u, --user string                  adds Basic authentication with USER:PASSWORD
  -A, --user-agent string            sets the User-Agent header sent to the server
      --variable stringArray         defines a variable
      --variables-file stringArray   defines variables from a properties file
  -v, --verbose                      turns on verbose output (alias of --verbosity verbose)
      --verbosity string             sets the verbosity level for debug logging
  -V, --version                      prints version, commit, build date and Go version
      --very-verbose                 turns on very verbose output, including HTTP and libcurl-style logs
```

## Global flags

```
      --color      colorize output
      --no-color   do not colorize output
```

Parent command: [sonde](sonde.md)
