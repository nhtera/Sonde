# Running request files from `go test`

The `engine` package runs request files from Go code, so API tests can live
next to the service they test and run with `go test`, against a real server
or an `httptest.Server`.

```go
package api_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/nhtera/sonde/engine"

	"example.com/myservice"
)

func TestHealth(t *testing.T) {
	srv := httptest.NewServer(myservice.Handler())
	defer srv.Close()

	r := engine.NewRunner(engine.Options{
		Variables: map[string]any{"base": srv.URL},
	})
	defer r.Close()

	res, err := r.RunFile(context.Background(), "testdata/health.hurl")
	if err != nil {
		t.Fatal(err) // unreadable file or setup failure
	}
	if res.ParseError != nil {
		t.Fatal(res.ParseError.Render())
	}
	for _, err := range res.Errors() { // the errors that decided the outcome
		t.Error(res.Redact(err.Render()))
	}
}
```

`testdata/health.hurl` uses the `base` variable:

```hurl
GET {{base}}/health
HTTP 200
[Asserts]
jsonpath "$.status" == "ok"
```

Notes:

- File paths in the request file (`file,` bodies, `output`) are confined to
  the file's directory unless `Options.FileRoot` says otherwise.
- `Options.Secrets` values are redacted from log events and by
  `UnitResult.Redact`; results hold raw values, so pass any text you print
  through `Redact`.
- An `*engine.Error` tells an assert failure (`Assert()`) from a runtime
  error, classifies itself with `Kind()` (`engine.ErrorAssertStatus`, ...)
  and locates itself with `Span()`. Captured values are `engine.Value`s
  with typed accessors (`Int()`, `Text()`, `List()`, ...).
- `Options.OnEvent` receives log events (for example verbose output) while
  the file runs.
- To run many files, `Runner.RunAll` runs them in parallel with isolated
  cookies and variables; see its example in the package documentation.
