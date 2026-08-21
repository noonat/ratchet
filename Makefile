# Ratchet. `make` prints this list; `make check` is what a gate runs.
#
# `fixtures` is deliberately absent from `check`: it reads journals from
# `journals/`, which is gitignored, so a fresh clone does not have them. A gate
# has to pass on a fresh clone.

GOFILES := $(shell find . -name '*.go' -not -path './vendor/*')
PKGS    := ./...
# Pinned, because a gate whose formatter floats reformats the repo on someone
# else's release day. Bump it deliberately, then run `make fmt`.
PRETTIER := prettier@3.9.6

.DEFAULT_GOAL := help

.PHONY: help
help: ## print this list
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk -F':.*?## ' '{printf "  %-8s %s\n", $$1, $$2}'

.PHONY: build
build: ## compile the binary
	go build -o ratchet ./cmd/ratchet

.PHONY: clean
clean: ## remove build output
	rm -f ratchet

.PHONY: fmt
fmt: ## rewrite source with gofmt and prose with prettier
	gofmt -w $(GOFILES)
	npx --yes $(PRETTIER) --write "**/*.md"

.PHONY: lint
lint: ## gofmt, vet, staticcheck
	@unformatted=$$(gofmt -l $(GOFILES)); \
	if [ -n "$$unformatted" ]; then \
		echo "not gofmt'd (run: make fmt):"; echo "$$unformatted"; exit 1; \
	fi
	go vet $(PKGS)
	@command -v staticcheck >/dev/null 2>&1 || { \
		echo "staticcheck not found; go install honnef.co/go/tools/cmd/staticcheck@latest"; \
		exit 1; \
	}
	staticcheck $(PKGS)
	@cp go.mod go.mod.tidycheck && cp go.sum go.sum.tidycheck; \
	go mod tidy; \
	if ! cmp -s go.mod go.mod.tidycheck || ! cmp -s go.sum go.sum.tidycheck; then \
		mv go.mod.tidycheck go.mod; mv go.sum.tidycheck go.sum; \
		echo "go.mod or go.sum is not tidy (run: go mod tidy)"; exit 1; \
	fi; \
	rm -f go.mod.tidycheck go.sum.tidycheck

.PHONY: prose
prose: ## check markdown formatting
	@command -v npx >/dev/null 2>&1 || { \
		echo "npx not found; install node to check prose formatting"; \
		exit 1; \
	}
	npx --yes $(PRETTIER) --check "**/*.md"

.PHONY: test
test: ## run the tests
	go test $(PKGS)

.PHONY: check
check: build lint prose test ## build, lint, prose, test — what a gate runs

.PHONY: fixtures
fixtures: ## rebuild testdata/fixtures.jsonl from journals/ (FORCE=1 to accept a change)
	go run ./cmd/ratchet-dev fixtures $(if $(FORCE),--force,)

.PHONY: replay
replay: ## report how often the applier agrees with the harness (DETAIL=1 for each disagreement)
	go run ./cmd/ratchet-dev replay $(if $(DETAIL),--disagreements,)
