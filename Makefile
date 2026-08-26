# agentlaunch — the single source of truth for every check in this module.
#
# CI (.github/workflows/ci.yml) invokes these targets rather than re-spelling
# the commands in YAML, so "green locally" and "green in CI" are the same
# claim. The target names clean/fmt/fmt-check/vet/lint/security/test/
# test-unit/pre-commit are a contract with the owner's /quality command —
# makefile_test.go pins them.
#
# This is a LIBRARY module: there is no binary, so no build, goreleaser or
# version-stamping targets. Everything else mirrors openrouter-launch, whose
# checks these packages were held to before the extraction and must still be.

PKG := github.com/teggen/agentlaunch

# Pinned tool versions. `make tools` installs them with the LOCAL Go
# toolchain on purpose: prebuilt analysis binaries break on toolchain skew.
# Keep these in step with openrouter-launch's Makefile — the two repos share
# a quality gate, and a version that drifts here reports different findings
# for the same code.
GOLANGCI_VERSION    := v2.12.2
GOSEC_VERSION       := v2.28.0
GOVULNCHECK_VERSION := latest
ACTIONLINT_VERSION  := latest

COVERAGE_MIN := 80
COVERPROFILE := coverage.out
COVERHTML    := coverage.html

GOBIN := $(shell go env GOPATH)/bin

# Landmine 8's isolated run strips the user's own PATH so really-installed
# agents (claude, pi, hermes, cline) become invisible. This matters MORE here
# than it did in the app: the launchers' findPath home-directory fallbacks
# live in this module now, so this is where a test that quietly depends on a
# real install gets caught.
GO_BIN_DIR    := $(shell dirname $$(command -v go))
ISOLATED_PATH ?= $(GO_BIN_DIR):/usr/bin:/bin

.DEFAULT_GOAL := help

.PHONY: help
help: ## List the available targets
	@awk 'BEGIN { FS = ":.*##" } \
	     /^[a-zA-Z0-9_-]+:.*##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 }' \
	     $(MAKEFILE_LIST)

.PHONY: clean
clean: ## Remove coverage output
	rm -f $(COVERPROFILE) $(COVERHTML)

## ---- format and lint -------------------------------------------------

.PHONY: fmt
fmt: ## Format all Go source in place
	gofmt -w .

.PHONY: fmt-check
fmt-check: ## Fail if any file is not gofmt-clean
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then \
	  echo "not gofmt-clean:"; echo "$$out"; \
	  echo "run: make fmt"; exit 1; \
	fi; \
	echo "gofmt: clean"

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint (requires v2 — see .golangci.yml)
	@test -x $(GOBIN)/golangci-lint || { echo "golangci-lint missing — run: make tools"; exit 1; }
	@$(GOBIN)/golangci-lint version 2>&1 | grep -qE 'version v?2\.' || { \
	  echo ".golangci.yml uses the v2 schema and the installed binary is v1."; \
	  echo "Upgrade the binary (make tools) — do NOT downgrade the config,"; \
	  echo "golangci-lint-action v9 rejects v1 outright."; exit 1; }
	$(GOBIN)/golangci-lint run ./...

.PHONY: lint-workflows
lint-workflows: ## Validate the GitHub Actions YAML
	@test -x $(GOBIN)/actionlint || { echo "actionlint missing — run: make tools"; exit 1; }
	$(GOBIN)/actionlint

# Not optional here. exec_windows.go is invisible to the linter on Linux, and
# it is one of the two files the noctx exclusion in .golangci.yml covers —
# so on this module's default GOOS the gate never sees the site whose
# rationale that exclusion states (Landmine 24).
.PHONY: lint-cross
lint-cross: ## Lint the build-tagged files the default GOOS never sees
	@test -x $(GOBIN)/golangci-lint || command -v golangci-lint >/dev/null || { \
	  echo "golangci-lint missing — run: make tools"; exit 1; }
	GOOS=windows $(or $(wildcard $(GOBIN)/golangci-lint),golangci-lint) run ./...
	GOOS=darwin  $(or $(wildcard $(GOBIN)/golangci-lint),golangci-lint) run ./...

## ---- security --------------------------------------------------------

# gosec's `test -x` guard is NOT redundant with the `-` prefix on its recipe
# line: `-` ignores "No such file or directory" just as happily as a finding,
# so without the guard an uninstalled gosec reports success having analysed
# nothing.
.PHONY: security
security: ## govulncheck + go mod verify (blocking); gosec (advisory)
	@test -x $(GOBIN)/govulncheck || { echo "govulncheck missing — run: make tools"; exit 1; }
	$(GOBIN)/govulncheck ./...
	go mod verify
	@echo "--- gosec (advisory: findings do not fail this target) ---"
	@test -x $(GOBIN)/gosec || { echo "gosec missing — run: make tools"; exit 1; }
	-@$(GOBIN)/gosec -quiet ./...

## ---- test ------------------------------------------------------------

.PHONY: test
test: ## Run the full test suite
	go test ./... -count=1

.PHONY: test-unit
test-unit: test ## Alias for `test` (this module has no unit/integration split)

# Scoped to ./... rather than one package, unlike the app's: the thing the
# race detector has to find lives HERE now — RunWait's goroutine and its
# two-channel select (Landmine 24's fork-and-wait path).
.PHONY: test-race
test-race: ## Race detector across the module
	go test ./... -race -count=1

.PHONY: test-isolated
test-isolated: ## Landmine 8: suite must stay green with real installs invisible
	HOME=$$(mktemp -d) PATH="$(ISOLATED_PATH)" go test ./... -count=1

.PHONY: cover
cover: ## Run the suite with coverage and print the total
	go test ./... -count=1 -covermode=atomic -coverprofile=$(COVERPROFILE)
	@go tool cover -func=$(COVERPROFILE) | tail -1

.PHONY: cover-html
cover-html: cover ## Write $(COVERHTML)
	go tool cover -html=$(COVERPROFILE) -o $(COVERHTML)
	@echo "wrote $(COVERHTML)"

.PHONY: cover-check
cover-check: cover ## Fail if total coverage is below $(COVERAGE_MIN)%
	@total=$$(go tool cover -func=$(COVERPROFILE) | tail -1 | awk '{print $$3}' | tr -d '%'); \
	echo "total coverage: $$total% (floor $(COVERAGE_MIN)%)"; \
	awk -v t="$$total" -v m="$(COVERAGE_MIN)" 'BEGIN { exit !(t + 0 >= m + 0) }' || { \
	  echo "coverage $$total% is below the $(COVERAGE_MIN)% floor"; exit 1; }

## ---- repository invariants -------------------------------------------

# `git status --porcelain` rather than openrouter-launch's `git diff
# --exit-code go.mod go.sum`, and the difference is not cosmetic. This module
# has no dependencies, so it has NO go.sum — and `git diff` given a path that
# does not exist in the working tree fails outright ("fatal: go.sum: no such
# path"), turning a tidy tree into a failed target. `git status --porcelain`
# takes the missing path in stride AND, unlike `git diff`, reports an
# UNTRACKED go.sum — which is the one thing this check most needs to catch
# here, since the first stray import is what would create one.
.PHONY: tidy-check
tidy-check: ## Fail if go.mod/go.sum are not tidy
	go mod tidy
	@if [ -n "$$(git status --porcelain -- go.mod go.sum)" ]; then \
	  git status --short -- go.mod go.sum; \
	  echo "go.mod/go.sum were not tidy — commit the result of 'go mod tidy'"; exit 1; \
	fi; \
	echo "go.mod/go.sum: tidy"

.PHONY: cross
cross: ## Cross-compile check (windows, darwin)
	GOOS=windows go build ./...
	GOOS=darwin go build ./...

.PHONY: writesites
writesites: ## Landmine 6: show every write primitive outside tests
	@grep -rn "os.WriteFile\|os.Create\|os.MkdirAll\|os.Rename\|OpenFile\|CreateTemp" \
	  --include="*.go" . | grep -v _test || true

## ---- tooling ---------------------------------------------------------

# GOTOOLCHAIN=auto applies only to BUILDING these tools; what builds and
# tests this module stays pinned to the go.mod floor via go-version-file.
.PHONY: tools
tools: ## Install the pinned lint/security tools with the local toolchain
	GOTOOLCHAIN=auto go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	GOTOOLCHAIN=auto go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)
	GOTOOLCHAIN=auto go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	$(MAKE) tools-actionlint
	@echo "installed into $(GOBIN)"

.PHONY: tools-actionlint
tools-actionlint: ## Install actionlint alone (with the local toolchain)
	GOTOOLCHAIN=auto go install github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

## ---- aggregates ------------------------------------------------------

.PHONY: pre-commit
pre-commit: clean fmt-check vet lint security test ## The /quality gate

.PHONY: ci
ci: fmt-check vet lint lint-cross lint-workflows tidy-check cross security test-race cover-check test-isolated ## Everything CI runs
