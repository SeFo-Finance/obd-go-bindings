PKG := github.com/SeFo-Finance/obd-go-bindings
ESCPKG := github.com\/SeFo-Finance\/obd-go-bindings

LINT_PKG := github.com/golangci/golangci-lint/cmd/golangci-lint
GOACC_PKG := github.com/ory/go-acc
GOIMPORTS_PKG := golang.org/x/tools/cmd/goimports
GOFUZZ_BUILD_PKG := github.com/dvyukov/go-fuzz/go-fuzz-build
GOFUZZ_PKG := github.com/dvyukov/go-fuzz/go-fuzz
GOFUZZ_DEP_PKG := github.com/dvyukov/go-fuzz/go-fuzz-dep

GO_BIN := ${GOPATH}/bin
LINT_BIN := $(GO_BIN)/golangci-lint
GOACC_BIN := $(GO_BIN)/go-acc
GOFUZZ_BUILD_BIN := $(GO_BIN)/go-fuzz-build
GOFUZZ_BIN := $(GO_BIN)/go-fuzz

COMMIT := $(shell git describe --tags --dirty)
COMMIT_HASH := $(shell git rev-parse HEAD)

GOBUILD := go build -v
GOINSTALL := go install -v
GOTEST := go test 

GOVERSION := $(shell go version | awk '{print $$3}')
GOFILES_NOVENDOR = $(shell find . -type f -name '*.go' -not -path "./vendor/*" -not -name "*pb.go" -not -name "*pb.gw.go" -not -name "*.pb.json.go")

RM := rm -f
CP := cp
MAKE := make
XARGS := xargs -L 1

# For the release, we want to remove the symbol table and debug information (-s)
# and omit the DWARF symbol table (-w). Also we clear the build ID.
RELEASE_LDFLAGS := $(call make_ldflags, $(RELEASE_TAGS), -s -w -buildid=)

# Linting uses a lot of memory, so keep it under control by limiting the number
# of workers if requested.
ifneq ($(workers),)
LINT_WORKERS = --concurrency=$(workers)
endif

LINT = $(LINT_BIN) run -v $(LINT_WORKERS)

GREEN := "\\033[0;32m"
NC := "\\033[0m"
define print
	echo $(GREEN)$1$(NC)
endef

# ============
# DEPENDENCIES
# ============
$(LINT_BIN):
	@$(call print, "Installing linter.")
	go install $(LINT_PKG)

$(GOACC_BIN):
	@$(call print, "Installing go-acc.")
	go install $(GOACC_PKG)


goimports:
	@$(call print, "Installing goimports.")
	go install $(GOIMPORTS_PKG)

$(GOFUZZ_BIN):
	@$(call print, "Installing go-fuzz.")
	go install $(GOFUZZ_PKG)

$(GOFUZZ_BUILD_BIN):
	@$(call print, "Installing go-fuzz-build.")
	go install $(GOFUZZ_BUILD_PKG)

$(GOFUZZ_DEP_BIN):
	@$(call print, "Installing go-fuzz-dep.")
	go install $(GOFUZZ_DEP_PKG)

# =========
# UTILITIES
# =========

fmt: goimports
	@$(call print, "Fixing imports.")
	goimports -w $(GOFILES_NOVENDOR)
	@$(call print, "Formatting source.")
	gofmt -l -w -s $(GOFILES_NOVENDOR)

lint: $(LINT_BIN)
	@$(call print, "Linting source.")
	$(LINT)

list:
	@$(call print, "Listing commands.")
	@$(MAKE) -qp | \
		awk -F':' '/^[a-zA-Z0-9][^$$#\/\t=]*:([^=]|$$)/ {split($$1,A,/ /);for(i in A)print A[i]}' | \
		grep -v Makefile | \
		sort

rpc:
	@$(call print, "Compiling protos.")
	cd ./obrpc; ./gen_protos_docker.sh

.PHONY: all \
	fmt \
	lint \
	list \
	rpc
