BUILDDIR ?= $(CURDIR)/build
TOOLS_DIR := tools

BABYLON_PKG := github.com/babylonlabs-io/babylon/cmd/babylond
WASMD_PKG   := github.com/CosmWasm/wasmd/cmd/wasmd
BCD_PKG     := github.com/babylonlabs-io/babylon-sdk/demo/cmd/bcd

GO_BIN := ${GOPATH}/bin
BTCD_BIN := $(GO_BIN)/btcd

DOCKER := $(shell which docker)
CUR_DIR := $(shell pwd)
MOCKS_DIR=$(CUR_DIR)/testutil/mocks
MOCKGEN_REPO=github.com/golang/mock/mockgen
MOCKGEN_VERSION=v1.6.0
MOCKGEN_CMD=go run ${MOCKGEN_REPO}@${MOCKGEN_VERSION}

ldflags := $(LDFLAGS)
build_tags := $(BUILD_TAGS)
build_args := $(BUILD_ARGS)

PACKAGES_E2E=$(shell go list ./... | grep '/itest')
# need to specify the full path to fix issue where logs won't stream to stdout
# due to multiple packages found
# context: https://github.com/golang/go/issues/24929
PACKAGES_E2E_OP=$(shell go list -tags=e2e_op ./... | grep '/itest/opstackl2')
PACKAGES_E2E_BCD=$(shell go list -tags=e2e_bcd ./... | grep '/itest/cosmwasm/bcd')

ifeq ($(LINK_STATICALLY),true)
	ldflags += -linkmode=external -extldflags "-Wl,-z,muldefs -static" -v
endif

ifeq ($(VERBOSE),true)
	build_args += -v
endif

BUILD_TARGETS := build install
BUILD_FLAGS := --tags "$(build_tags)" --ldflags '$(ldflags)'

# Update changelog vars
ifneq (,$(SINCE_TAG))
	sinceTag := --since-tag $(SINCE_TAG)
endif
ifneq (,$(UPCOMING_TAG))
	upcomingTag := --future-release $(UPCOMING_TAG)
endif

all: build install

build: BUILD_ARGS := $(build_args) -o $(BUILDDIR)

$(BUILD_TARGETS): go.sum $(BUILDDIR)/
	CGO_CFLAGS="-O -D__BLST_PORTABLE__" go $@ -mod=readonly $(BUILD_FLAGS) $(BUILD_ARGS) ./...

$(BUILDDIR)/:
	mkdir -p $(BUILDDIR)/

build-docker:
	$(DOCKER) build --tag babylonlabs-io/finality-provider -f Dockerfile \
		$(shell git rev-parse --show-toplevel)

.PHONY: build build-docker

.PHONY: lint
lint:
	golangci-lint run

.PHONY: test
test:
	go test ./...

install-babylond:
	cd $(TOOLS_DIR); \
	go install -trimpath $(BABYLON_PKG)

install-wasmd:
	cd $(TOOLS_DIR); \
	go install -trimpath $(WASMD_PKG)

install-bcd:
	cd $(TOOLS_DIR); \
	go install -trimpath $(BCD_PKG)

.PHONY: clean-e2e test-e2e test-e2e-babylon test-e2e-babylon-ci test-e2e-wasmd test-e2e-bcd test-e2e-op test-e2e-op-ci

# Clean up environments by stopping processes and removing data directories
clean-e2e:
	@pids=$$(ps aux | grep -E 'babylond start|wasmd start|bcd start' | grep -v grep | awk '{print $$2}' | tr '\n' ' '); \
	if [ -n "$$pids" ]; then \
		echo $$pids | xargs kill; \
		echo "Killed processes $$pids"; \
	else \
		echo "No processes to kill"; \
	fi
	rm -rf ~/.babylond ~/.wasmd ~/.bcd

# Main test target that runs all e2e tests
test-e2e: test-e2e-babylon test-e2e-wasmd test-e2e-bcd test-e2e-op

test-e2e-babylon: clean-e2e install-babylond
	@go test -mod=readonly -timeout=25m -v $(PACKAGES_E2E) -count=1 --tags=e2e_babylon

test-e2e-babylon-ci: clean-e2e install-babylond
	go test -list . ./itest/babylon --tags=e2e_babylon | grep Test \
	| circleci tests run --command \
	"xargs go test -mod=readonly -timeout=25m -v $(PACKAGES_E2E) -count=1 --tags=e2e_babylon --run" \
	--split-by=name --timings-type=name

test-e2e-bcd: clean-e2e install-babylond install-bcd
	@go test -race -mod=readonly -timeout=25m -v $(PACKAGES_E2E_BCD) -count=1 --tags=e2e_bcd

test-e2e-wasmd: clean-e2e install-babylond install-wasmd
	@go test -mod=readonly -timeout=25m -v $(PACKAGES_E2E) -count=1 --tags=e2e_wasmd

test-e2e-op: clean-e2e install-babylond
	@go test -mod=readonly -timeout=25m -v $(PACKAGES_E2E_OP) -count=1 --tags=e2e_op

FILTER ?= .
test-e2e-op-filter: clean-e2e install-babylond
	@go test -mod=readonly -timeout=25m -v $(PACKAGES_E2E_OP) -count=1 --tags=e2e_op --run ^$(FILTER)$

test-e2e-op-ci: clean-e2e install-babylond
	go test -list . ./itest/opstackl2 --tags=e2e_op | grep Test \
	| circleci tests run --command \
	"xargs go test -race -mod=readonly -timeout=25m -v $(PACKAGES_E2E_OP) -count=1 --tags=e2e_op --run" \
	--split-by=name --timings-type=name

DEVNET_REPO_URL := https://github.com/babylonlabs-io/op-e2e-devnet
TARGET_DIR := ./itest/opstackl2/devnet-data

.PHONY: op-e2e-devnet
op-e2e-devnet:
	@rm -rf $(TARGET_DIR)
	@mkdir -p $(TARGET_DIR)
	@git clone $(DEVNET_REPO_URL) $(TARGET_DIR)
	@echo "Devnet data downloaded to $(TARGET_DIR)"

###############################################################################
###                                Protobuf                                 ###
###############################################################################

proto-all: proto-gen

proto-gen:
	make -C eotsmanager proto-gen
	make -C finality-provider proto-gen

.PHONY: proto-gen

mock-gen:
	mkdir -p $(MOCKS_DIR)
	$(MOCKGEN_CMD) -source=clientcontroller/api/interface.go -package mocks -destination $(MOCKS_DIR)/clientcontroller.go

.PHONY: mock-gen

update-changelog:
	@echo ./scripts/update_changelog.sh $(sinceTag) $(upcomingTag)
	./scripts/update_changelog.sh $(sinceTag) $(upcomingTag)

.PHONY: update-changelog
