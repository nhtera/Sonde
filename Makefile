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

.PHONY: apicheck build test race lint vuln fuzz-smoke conformance conformance-update snapshot license-check headers licenses docs tools clean

build: ## Build bin/sonde (static, trimmed)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN)/sonde ./cmd/sonde

test: ## Run unit tests
	go test ./...

race: ## Run unit tests with the race detector
	go test -race ./...

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

snapshot: ## Local GoReleaser snapshot build into dist/
	goreleaser release --snapshot --clean

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
