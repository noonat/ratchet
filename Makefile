# Ratchet. `make` prints this list; `make check` is what a gate runs.
#
# `corpus` is deliberately absent from `check`: it reads journals from
# `journals/`, which is gitignored, so a fresh clone does not have them. A gate
# has to pass on a fresh clone.

GOFILES := $(shell find . -name '*.go' -not -path './vendor/*')
PKGS    := ./...

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
fmt: ## rewrite source with gofmt
	gofmt -w $(GOFILES)

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

.PHONY: test
test: ## run the tests
	go test $(PKGS)

.PHONY: check
check: build lint test ## build, lint, test — what a gate runs
