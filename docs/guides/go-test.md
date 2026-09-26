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
		t.Fatal(res.ParseError.Render(res.File, res.Source))
	}
	for _, e := range res.Entries {
		for _, err := range e.Errors {
			if !e.Retried {
				t.Error(r.Redact(err.Render(res.File, string(res.Source), e.Line)))
			}
		}
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
  `Runner.Redact`; results hold raw values, so pass any text you print
  through `Redact`.
- `Options.OnEvent` receives log events (for example verbose output) while
  the file runs.
- To run many files, `Runner.RunAll` runs them in parallel with isolated
  cookies and variables; see its example in the package documentation.
