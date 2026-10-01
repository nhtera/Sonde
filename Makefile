# Sonde developer tasks. Tools are pinned and installed into ./bin.

GOLANGCI_LINT_VERSION := v2.14.0
GOVULNCHECK_VERSION   := v1.8.0
GO_LICENSES_VERSION   := v2.0.1
APIDIFF_VERSION       := v0.0.0-20260908205506-85c1c2202aba
ALLOWED_LICENSES      := Apache-2.0,MIT,BSD-2-Clause,BSD-3-Clause,ISC

BIN      := $(CURDIR)/bin
# Build tools with the repo toolchain (go.mod `toolchain`), not the tools' own minimum.
GO_TOOLCHAIN = $(shell go env GOVERSION)
GO_INSTALL   = GOBIN=$(BIN) GOTOOLCHAIN=$(GO_TOOLCHAIN) go install
PKG      := github.com/nhtera/sonde/internal/cli
VERSION  ?= dev
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE     ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -s -w -X $(PKG).version=$(VERSION) -X $(PKG).commit=$(COMMIT) -X $(PKG).date=$(DATE)
FUZZTIME ?= 10s

.PHONY: apicheck bench verify-install build test race test-grpc-interop lint vuln fuzz-smoke conformance conformance-update snapshot license-check headers licenses docs tools clean desktop-tools desktop-bindings desktop-check desktop-record desktop-e2e desktop-vuln lint-desktop-native tag-guards

build: ## Build bin/sonde (static, trimmed)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN)/sonde ./cmd/sonde

test: ## Run unit tests
	go test ./...

race: ## Run unit tests with the race detector
	go test -race ./...

test-grpc-interop: ## Run gRPC calls against grpc-go servers (separate module under testdata/grpcinterop)
	cd testdata/grpcinterop && go test -race ./...

lint: $(BIN)/golangci-lint ## Run golangci-lint (linters + formatters)
	$(BIN)/golangci-lint run ./...

vuln: $(BIN)/govulncheck ## Scan for known vulnerabilities
	$(BIN)/govulncheck ./...

fuzz-smoke: ## Run every Fuzz target briefly (FUZZTIME=10s)
	@for pkg in $$(go list ./...); do \
	  list=$$(go test -list '^Fuzz' $$pkg) || exit 1; \
	  for fn in $$(echo "$$list" | grep '^Fuzz'); do \
	    echo "fuzz $$pkg $$fn"; \
	    go test -run='^$$' -fuzz="^$$fn$$" -fuzztime=$(FUZZTIME) $$pkg || exit 1; \
	  done; \
	done

conformance: ## Run the Hurl conformance suite against SONDE_CONFORMANCE_BIN (default: build ./cmd/sonde)
	SONDE_CONFORMANCE=1 go test ./internal/conformance -count=1 -v -timeout 60m

conformance-update: ## Rerun the conformance suite and rewrite internal/conformance/manifest.yaml (CONFORMANCE_ALLOW_DEMOTE=1 to allow demotions)
	SONDE_CONFORMANCE=1 SONDE_CONFORMANCE_UPDATE=1 CONFORMANCE_ALLOW_DEMOTE=$(CONFORMANCE_ALLOW_DEMOTE) go test ./internal/conformance -count=1 -v -timeout 60m

snapshot: ## Local GoReleaser snapshot build into dist/ (sign/sbom need cosign/syft, installed only in CI's release job)
	goreleaser release --snapshot --clean --skip=sign,sbom

license-check: headers licenses ## SPDX headers + dependency license allowlist

headers: ## Check SPDX headers (and self-test the checker)
	scripts/check-license-headers-test.sh
	scripts/check-license-headers.sh

licenses: $(BIN)/go-licenses ## Check dependency licenses against the allowlist
	$(BIN)/go-licenses check ./... --allowed_licenses=$(ALLOWED_LICENSES)

docs: ## Regenerate docs/compat.md and docs/cli from the command tree
	go test ./internal/docs -run TestCompatUpToDate -update
	go test ./internal/cli -run TestCLIReferenceUpToDate -update

apicheck: $(BIN)/apidiff ## Fail on incompatible public API changes since .api-baseline (BASE=ref to override)
	scripts/apicheck.sh $(BASE)

bench: ## Local benchmarks vs other installed HTTP runners (report-only; see docs/benchmarks.md)
	scripts/bench.sh

verify-install: ## Verify a published release installs and verifies (VERSION=vX.Y.Z)
	scripts/verify-install.sh $(VERSION)

tag-guards: ## Check that no release tag namespace (editors/vscode/*, desktop/*) leaks into the CLI's tag lookups
	scripts/check-tag-guards.sh

# The desktop app is a nested module (desktop/go.mod) with a Wails runtime;
# the root module never imports it. Its Go checks use the CGO-free builds
# (server mode and the test harness); the native window build is checked
# on macOS runners.
DESKTOP_TAGS := server server,production server,e2eharness

desktop-tools: ## Install the pinned desktop tools (wails3, task) into ./bin and verify them
	BIN=$(BIN) desktop/scripts/tools.sh install

desktop-bindings: desktop-tools ## Generate the frontend's TypeScript bindings (not committed) from the desktop services
	cd desktop && CGO_ENABLED=0 $(BIN)/wails3 generate bindings -clean=true -ts -i -silent -f "-tags server"

desktop-check: $(BIN)/golangci-lint $(BIN)/go-licenses desktop-bindings ## Desktop module: tidy, licenses, vet, tests, lint; frontend: install (no scripts), licenses, version, lint, typecheck, unit tests
	cd desktop && go mod tidy -diff
	cd desktop && $(BIN)/go-licenses check ./... --allowed_licenses=$(ALLOWED_LICENSES) --ignore github.com/nhtera/sonde
	@test -f desktop/frontend/dist/index.html || { mkdir -p desktop/frontend/dist && echo '<!doctype html><title>Sonde</title>' > desktop/frontend/dist/index.html; }
	cd desktop && for tags in $(DESKTOP_TAGS); do \
	  CGO_ENABLED=0 go vet -tags $$tags ./... && CGO_ENABLED=0 go test -tags $$tags ./... && $(BIN)/golangci-lint run --build-tags $$tags ./... || exit 1; \
	done
	npm --prefix desktop/frontend ci --ignore-scripts --no-audit --no-fund
	node desktop/scripts/check-install-scripts.mjs
	cd desktop && node scripts/check-npm-licenses.mjs && node scripts/version.mjs && scripts/verify-artifacts_test.sh && node --test scripts/*.test.mjs
	npm --prefix desktop/frontend run lint
	npm --prefix desktop/frontend run typecheck
	npm --prefix desktop/frontend test

desktop-record: ## Re-record the shop-api run events that the frontend's run component tests replay
	@test -f desktop/frontend/dist/index.html || { mkdir -p desktop/frontend/dist && echo '<!doctype html><title>Sonde</title>' > desktop/frontend/dist/index.html; }
	cd desktop && SONDE_RECORD_DIR=$(CURDIR)/desktop/frontend/src/components/run/testdata CGO_ENABLED=0 go test -tags server -run TestRecordRunEvents -count=1 .

desktop-vuln: $(BIN)/govulncheck ## Scan the desktop module (server build: the window build adds only platform webview code) for known vulnerabilities
	cd desktop && $(BIN)/govulncheck -tags server ./...

lint-desktop-native: $(BIN)/golangci-lint ## Lint the desktop module's native window build (needs cgo and the platform webview)
	cd desktop && $(BIN)/golangci-lint run ./...

desktop-e2e: desktop-bindings ## Build server mode, the test-only harness and the fixture API, then run the browser tests (browsers: npx playwright install)
	npm --prefix desktop/frontend run build
	cd desktop && CGO_ENABLED=0 go build -tags server,production -o bin/sonde-desktop-server .
	npm --prefix desktop/frontend run build:harness
	cd desktop && CGO_ENABLED=0 go build -tags server,e2eharness -o bin/sonde-desktop-harness .
	cd desktop && CGO_ENABLED=0 go build -o bin/fixture-server ./cmd/fixture-server
	CGO_ENABLED=0 go build -o desktop/bin/sonde ./cmd/sonde
	cd desktop/frontend && npx playwright test

tools: $(BIN)/golangci-lint $(BIN)/govulncheck $(BIN)/go-licenses $(BIN)/apidiff ## Install pinned tools into ./bin

$(BIN)/golangci-lint:
	$(GO_INSTALL) github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(BIN)/govulncheck:
	$(GO_INSTALL) golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

$(BIN)/apidiff:
	$(GO_INSTALL) golang.org/x/exp/cmd/apidiff@$(APIDIFF_VERSION)

$(BIN)/go-licenses:
	$(GO_INSTALL) github.com/google/go-licenses/v2@$(GO_LICENSES_VERSION)

clean:
	rm -rf $(BIN) dist coverage.*
