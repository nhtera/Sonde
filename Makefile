# Sonde developer tasks. Tools are pinned and installed into ./bin.

GOLANGCI_LINT_VERSION := v2.14.0
GOVULNCHECK_VERSION   := v1.8.0
GO_LICENSES_VERSION   := v2.0.1
APIDIFF_VERSION       := v0.0.0-20260908205506-85c1c2202aba
ALLOWED_LICENSES      := Apache-2.0,MIT,BSD-2-Clause,BSD-3-Clause,ISC
# Single libraries accepted under another license, used unmodified (see
# NOTICE): go-uuid (MPL-2.0) comes with the Kerberos client for --negotiate.
LICENSE_EXCEPTIONS    := --ignore github.com/hashicorp/go-uuid

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

.PHONY: apicheck bench verify-install build test race test-grpc-interop lint vuln fuzz-smoke conformance conformance-update conformance-next conformance-next-update snapshot license-check headers licenses docs tools clean desktop-tools desktop-bindings desktop-check desktop-record desktop-e2e desktop-vuln lint-desktop-native tag-guards site site-dev site-check site-screens

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

# Both conformance targets bind the same ports (8000-8004, 3128), so a lock
# directory refuses a second concurrent run instead of letting it collide.
CONFORMANCE_LOCK = $${TMPDIR:-/tmp}/sonde-conformance.lock
# Passed through the environment, never re-quoted in a recipe: a demotion
# reason may hold backticks or $ that a shell would otherwise expand.
export CONFORMANCE_DEMOTE_ONLY CONFORMANCE_DEMOTE_REASON CONFORMANCE_ALLOW_DEMOTE
define conformance_run
	@lock="$(CONFORMANCE_LOCK)"; \
	mkdir "$$lock" 2>/dev/null || { echo "conformance: $$lock exists: another conformance run is using its ports (remove the directory if it is stale)" >&2; exit 1; }; \
	trap 'rmdir "$$lock" 2>/dev/null' EXIT INT TERM; \
	SONDE_CONFORMANCE=1 $(1) go test ./internal/conformance -count=1 -v -timeout 60m
endef

conformance: ## Run every conformance lane (hurl, hurlfmt, pty) against SONDE_CONFORMANCE_BIN (default: build ./cmd/sonde)
	$(call conformance_run,)

conformance-update: ## Rerun the suite and rewrite internal/conformance/manifest.yaml (demote with CONFORMANCE_DEMOTE_ONLY=paths CONFORMANCE_DEMOTE_REASON=why)
	$(call conformance_run,SONDE_CONFORMANCE_UPDATE=1)

conformance-next: ## Run the suite against the upstream commit pinned in internal/conformance/manifest-next.yaml (synced to the user cache)
	$(call conformance_run,SONDE_CONFORMANCE_NEXT=1)

conformance-next-update: ## Rerun the pinned snapshot and rewrite internal/conformance/manifest-next.yaml
	$(call conformance_run,SONDE_CONFORMANCE_NEXT=1 SONDE_CONFORMANCE_UPDATE=1)

snapshot: ## Local GoReleaser snapshot build into dist/ (sign/sbom need cosign/syft, installed only in CI's release job)
	goreleaser release --snapshot --clean --skip=sign,sbom

license-check: headers licenses ## SPDX headers + dependency license allowlist

headers: ## Check SPDX headers (and self-test the checker)
	scripts/check-license-headers-test.sh
	scripts/check-license-headers.sh

licenses: $(BIN)/go-licenses ## Check dependency licenses against the allowlist
	$(BIN)/go-licenses check ./... --allowed_licenses=$(ALLOWED_LICENSES) $(LICENSE_EXCEPTIONS)

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
	cd desktop && $(BIN)/go-licenses check ./... --allowed_licenses=$(ALLOWED_LICENSES) --ignore github.com/nhtera/sonde $(LICENSE_EXCEPTIONS)
	@test -f desktop/frontend/dist/index.html || { mkdir -p desktop/frontend/dist && echo '<!doctype html><title>Sonde</title>' > desktop/frontend/dist/index.html; }
	cd desktop && for tags in $(DESKTOP_TAGS); do \
	  CGO_ENABLED=0 go vet -tags $$tags ./... && CGO_ENABLED=0 go test -tags $$tags ./... && $(BIN)/golangci-lint run --build-tags $$tags ./... || exit 1; \
	done
	npm --prefix desktop/frontend ci --ignore-scripts --no-audit --no-fund
	node desktop/scripts/check-install-scripts.mjs
	cd desktop && node scripts/check-npm-licenses.mjs && node scripts/version.mjs && scripts/verify-artifacts_test.sh && scripts/check-app-zip_test.sh && node --test scripts/*.test.mjs
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

# The newest stable desktop tag (a pre-release has a - suffix), or the newest
# tag when none is stable: the version the screenshots' status bar shows.
site-screens: desktop-bindings ## Capture the website's screenshots from the desktop harness, then optimize them into site/ (browsers: npx playwright install)
	v=$$(git tag --list 'desktop/v*' --sort=-v:refname | grep -v -- '-' | head -n 1); \
	[ -n "$$v" ] || v=$$(git tag --list 'desktop/v*' --sort=-v:refname | head -n 1); \
	[ -n "$$v" ] || { echo "no desktop/v* tag: the status bar needs a version" >&2; exit 1; }; \
	v=$${v#desktop/v}; \
	SONDE_VERSION=$$v npm --prefix desktop/frontend run build:harness && \
	(cd desktop && CGO_ENABLED=0 go build -tags server,e2eharness -o bin/sonde-desktop-harness . && CGO_ENABLED=0 go build -o bin/fixture-server ./cmd/fixture-server) && \
	(cd desktop/frontend && E2E_MARKETING=1 npx playwright test --project=marketing) && \
	SONDE_VERSION=$$v npm --prefix site run screens

# The website (site/): landing page and docs, built from docs/. Node >= 22.18.
site: ## Build the website into site/.cloudflare/output/v0 (every page prerendered)
	npm --prefix site ci --ignore-scripts --no-audit --no-fund
	npm --prefix site run build

site-dev: ## Serve the website locally with live reload (http://localhost:3000)
	npm --prefix site run dev

site-check: ## Website: install (no scripts), install-script and license checks, audit, lint, typecheck, tests, build
	npm --prefix site ci --ignore-scripts --no-audit --no-fund
	npm --prefix site run check:install-scripts
	npm --prefix site run check:licenses
	npm --prefix site audit --audit-level=high
	npm --prefix site run lint
	npm --prefix site run typecheck
	npm --prefix site test
	npm --prefix site run build
	npm --prefix site run check-links

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
