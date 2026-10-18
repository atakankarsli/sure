BINARY_NAME := sure
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

GO := go
export CGO_ENABLED := 0

.PHONY: all
all: build

.PHONY: build
build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .

.PHONY: test
test:
	$(GO) test ./... -count=1

# The race detector needs cgo; it is the only target that enables it.
.PHONY: test-race
test-race:
	CGO_ENABLED=1 $(GO) test -race ./... -count=1

.PHONY: test-coverage
test-coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

.PHONY: lint
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		$(GO) vet ./...; \
	fi

.PHONY: format
format:
	@if command -v gofumpt >/dev/null 2>&1; then \
		gofumpt -w .; \
	else \
		gofmt -s -w .; \
	fi

.PHONY: format-check
format-check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "Formatting issues:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

# Cross-builds every release target without cgo.
.PHONY: build-all
build-all:
	@for os in darwin linux; do for arch in amd64 arm64; do \
		echo "$$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME)_$${os}_$${arch} . || exit 1; \
	done; done

.PHONY: install-hooks
install-hooks:
	git config core.hooksPath .githooks
	chmod +x .githooks/pre-commit

.PHONY: deps
deps:
	$(GO) mod download
	$(GO) mod tidy

.PHONY: clean
clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/
	rm -f coverage.out coverage.html

.PHONY: dev
dev: format lint test build

.PHONY: help
help:
	@echo "Targets:"
	@echo "  build          Build the binary (CGO_ENABLED=0)"
	@echo "  test           Run all tests offline"
	@echo "  test-race      Run all tests with the race detector"
	@echo "  test-coverage  Tests with coverage report"
	@echo "  lint           golangci-lint, or go vet"
	@echo "  format         Format code"
	@echo "  format-check   Fail on unformatted files"
	@echo "  build-all      Cross-build darwin/linux amd64/arm64 into dist/"
	@echo "  install-hooks  Use .githooks/pre-commit"
	@echo "  deps           Download and tidy modules"
	@echo "  clean          Remove build output"
	@echo "  dev            format + lint + test + build"
