# See https://tech.davis-hansson.com/p/make/
SHELL := bash
.DELETE_ON_ERROR:
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := all
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules
MAKEFLAGS += --no-print-directory
BIN := .tmp/bin
COPYRIGHT_YEARS := 2022
LICENSE_IGNORE := -e internal/proto/grpc -e internal/interop/grpc -e web/spec/grpc-web.spec.ts
# Set to use a different compiler. For example, `GO=go1.18rc1 make test`.
GO ?= go
# Set to use latest commit of `connect-go`, `connect-web`, and/or `protobuf-es` for docker compose test. For example, `TEST_LATEST_COMMIT=true make test-docker-compose`
TEST_LATEST_COMMIT ?= false

.PHONY: help
help: ## Describe useful make targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "%-30s %s\n", $$1, $$2}'

.PHONY: all
all: ## Build, test, and lint (default)
	$(MAKE) test
	$(MAKE) lint

.PHONY: clean
clean: ## Delete intermediate build artifacts
	@# -X only removes untracked files, -d recurses into directories, -f actually removes files/dirs
	git clean -Xdf

.PHONY: test
test: build ## Run unit tests
	$(GO) test -vet=off -race -cover ./...

.PHONY: shorttest
shorttest: build ## Run unit tests
	$(GO) test -test.short -vet=off -race -cover ./...

.PHONY: build
build: generate ## Build all packages
	$(GO) build ./...

.PHONY: lint
lint: $(BIN)/golangci-lint $(BIN)/buf ## Lint Go and protobuf
	test -z "$$($(BIN)/buf format -d . | tee /dev/stderr)"
	$(GO) vet ./...
	$(BIN)/golangci-lint run
	$(BIN)/buf lint
	cd web; npm install; npm run lint

.PHONY: lintfix
lintfix: $(BIN)/golangci-lint $(BIN)/buf ## Automatically fix some lint errors
	$(BIN)/golangci-lint run --fix
	$(BIN)/buf format -w .

.PHONY: generate
generate: $(BIN)/buf $(BIN)/protoc-gen-go $(BIN)/protoc-gen-connect-go $(BIN)/protoc-gen-go-grpc $(BIN)/protoc-gen-es $(BIN)/protoc-gen-connect-web $(BIN)/license-header ## Regenerate code and licenses
	rm -rf internal/gen
	rm -rf web/gen
	PATH=$(BIN) $(BIN)/buf generate
	@# We want to operate on a list of modified and new files, excluding
	@# deleted and ignored files. git-ls-files can't do this alone. comm -23 takes
	@# two files and prints the union, dropping lines common to both (-3) and
	@# those only in the second file (-2). We make one git-ls-files call for
	@# the modified, cached, and new (--others) files, and a second for the
	@# deleted files.
	comm -23 \
		<(git ls-files --cached --modified --others --no-empty-directory --exclude-standard | sort -u | grep -v $(LICENSE_IGNORE) ) \
		<(git ls-files --deleted | sort -u) | \
		xargs $(BIN)/license-header \
			--license-type apache \
			--copyright-holder "Buf Technologies, Inc." \
			--year-range "$(COPYRIGHT_YEARS)"

.PHONY: upgrade
upgrade: ## Upgrade dependencies
	go get -u -t ./... && go mod tidy -v

.PHONY: checkgenerate
checkgenerate:
	@# Used in CI to verify that `make generate` doesn't produce a diff.
	test -z "$$(git status --porcelain | tee /dev/stderr)"

$(BIN)/buf: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/bufbuild/buf/cmd/buf@v1.2.1

$(BIN)/license-header: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install \
		  github.com/bufbuild/buf/private/pkg/licenseheader/cmd/license-header@v1.2.1

$(BIN)/golangci-lint: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.45.2

$(BIN)/protoc-gen-connect-go: Makefile
	@mkdir -p $(@D)
	@# Pinned by go.mod.
	GOBIN=$(abspath $(@D)) $(GO) install github.com/bufbuild/connect-go/cmd/protoc-gen-connect-go

$(BIN)/protoc-gen-go-grpc: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2.0

$(BIN)/protoc-gen-go: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@v1.27.1

$(BIN)/protoc-gen-es: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/bufbuild/protobuf-es/cmd/protoc-gen-es@v0.0.0-20220404100843-2bf5c0f2d1c3

$(BIN)/protoc-gen-connect-web: Makefile
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) $(GO) install github.com/bufbuild/connect-web/cmd/protoc-gen-connect-web@v0.0.0-20220407075159-6fda16455846

docker-compose-clean:
	-docker-compose down --rmi local --remove-orphans
	# clean up errors are ignored

test-docker-compose: docker-compose-clean
	@# The NPM_TOKEN checking can be remove when connect-web and protobuf-es become public
ifeq ($(NPM_TOKEN),)
	$(error "$$NPM_TOKEN must be set to run docker tests")
endif
	TEST_LATEST_COMMIT=$(TEST_LATEST_COMMIT) docker-compose run client-connect-to-server-connect
	TEST_LATEST_COMMIT=$(TEST_LATEST_COMMIT) docker-compose run client-connect-to-server-grpc
	TEST_LATEST_COMMIT=$(TEST_LATEST_COMMIT) docker-compose run client-grpc-to-server-connect
	TEST_LATEST_COMMIT=$(TEST_LATEST_COMMIT) docker-compose run client-grpc-to-server-grpc
	TEST_LATEST_COMMIT=$(TEST_LATEST_COMMIT) docker-compose run client-grpc-web-to-server-connect-h1
	TEST_LATEST_COMMIT=$(TEST_LATEST_COMMIT) docker-compose run client-grpc-web-to-envoy-server-connect
	TEST_LATEST_COMMIT=$(TEST_LATEST_COMMIT) docker-compose run client-grpc-web-to-envoy-server-grpc
	TEST_LATEST_COMMIT=$(TEST_LATEST_COMMIT) docker-compose run client-connect-web-to-server-connect-h1
	TEST_LATEST_COMMIT=$(TEST_LATEST_COMMIT) docker-compose run client-connect-web-to-envoy-server-connect
	TEST_LATEST_COMMIT=$(TEST_LATEST_COMMIT) docker-compose run client-connect-web-to-envoy-server-grpc
	$(MAKE) docker-compose-clean
