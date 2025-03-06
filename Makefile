#   Copyright The containerd Authors.

#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at

#       http://www.apache.org/licenses/LICENSE-2.0

#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.

# -----------------------------------------------------------------------------
# Portions from https://github.com/kubernetes-sigs/cri-tools/blob/v1.19.0/Makefile
# Copyright The Kubernetes Authors.
# Licensed under the Apache License, Version 2.0
# -----------------------------------------------------------------------------

##########################
# Configuration
##########################
PACKAGE := "github.com/containerd/nerdctl/v2"
ORG_PREFIXES := "github.com/containerd"

DOCKER ?= docker
GO ?= go
GOOS ?= $(shell $(GO) env GOOS)
ifeq ($(GOOS),windows)
	BIN_EXT := .exe
endif

# distro builders might want to override these
PREFIX  ?= /usr/local
BINDIR  ?= $(PREFIX)/bin
DATADIR ?= $(PREFIX)/share
DOCDIR  ?= $(DATADIR)/doc

BINARY ?= "nerdctl"
MAKEFILE_DIR := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
VERSION ?= $(shell git -C $(MAKEFILE_DIR) describe --match 'v[0-9]*' --dirty='.m' --always --tags 2>/dev/null \
	|| echo "no_git_information")
VERSION_TRIMMED := $(VERSION:v%=%)
REVISION ?= $(shell git -C $(MAKEFILE_DIR) rev-parse HEAD 2>/dev/null || echo "no_git_information")$(shell \
	if ! git -C $(MAKEFILE_DIR) diff --no-ext-diff --quiet --exit-code 2>/dev/null; then echo .m; fi)
LINT_COMMIT_RANGE ?= main..HEAD
GO_BUILD_LDFLAGS ?= -s -w
GO_BUILD_FLAGS ?=

##########################
# Helpers
##########################
ARCH := amd64
ifeq ($(shell uname -m), aarch64)
	ARCH = arm64
endif
OS := linux
ifeq ($(shell uname -s), Darwin)
	OS = darwin
endif
ifeq ($(shell uname -s), FreeBSD)
	OS = freebsd
endif

ifdef VERBOSE
	VERBOSE_FLAG := -v
	VERBOSE_FLAG_LONG := --verbose
endif

export GO_BUILD=CGO_ENABLED=0 GOOS=$(GOOS) $(GO) -C $(MAKEFILE_DIR) build -ldflags "$(GO_BUILD_LDFLAGS) $(VERBOSE_FLAG) -X $(PACKAGE)/pkg/version.Version=$(VERSION) -X $(PACKAGE)/pkg/version.Revision=$(REVISION)"

ifndef NO_COLOR
    NC := \033[0m
    GREEN := \033[1;32m
    ORANGE := \033[1;33m
endif

recursive_wildcard=$(wildcard $1$2) $(foreach e,$(wildcard $1*),$(call recursive_wildcard,$e/,$2))

define title
	@printf "$(GREEN)____________________________________________________________________________________________________\n"
	@printf "$(GREEN)%*s\n" $$(( ( $(shell echo "🤓$(1) 🤓" | wc -c ) + 100 ) / 2 )) "🤓$(1) 🤓"
	@printf "$(GREEN)____________________________________________________________________________________________________\n$(ORANGE)"
endef

define footer
	@printf "$(GREEN)> %s: done!\n" "$(1)"
	@printf "$(GREEN)____________________________________________________________________________________________________\n$(NC)"
endef

##########################
# High-level tasks definitions
##########################
all: binaries

lint: lint-go-all lint-imports lint-yaml lint-shell lint-commits lint-mod lint-licenses-all

fix: fix-mod fix-imports fix-go-all

# TODO: fix race task and add it
test: test-unit # test-unit-race test-unit-bench

help:
	@echo "Usage: make <target>"
	@echo
	@echo " * 'lint' - Run linters against codebase."
	@echo " * 'fix' - Automatically fixes imports, modules, and simple formatting."
	@echo " * 'test' - Run basic unit testing."
	@echo " * 'binaries' - Build nerdctl."
	@echo " * 'install' - Install binaries to system locations."
	@echo " * 'clean' - Clean artifacts."

##########################
# Building and installation tasks
##########################
binaries: $(CURDIR)/_output/$(BINARY)$(BIN_EXT)

$(CURDIR)/_output/$(BINARY)$(BIN_EXT):
	$(call title, $@)
	$(GO_BUILD) $(GO_BUILD_FLAGS) $(VERBOSE_FLAG) -o $(CURDIR)/_output/$(BINARY)$(BIN_EXT) ./cmd/nerdctl
	$(call footer, $@)

install:
	$(call title, $@)
	install -D -m 755 $(CURDIR)/_output/$(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)
	install -D -m 755 $(MAKEFILE_DIR)/extras/rootless/containerd-rootless.sh $(DESTDIR)$(BINDIR)/containerd-rootless.sh
	install -D -m 755 $(MAKEFILE_DIR)/extras/rootless/containerd-rootless-setuptool.sh $(DESTDIR)$(BINDIR)/containerd-rootless-setuptool.sh
	install -D -m 644 -t $(DESTDIR)$(DOCDIR)/nerdctl $(MAKEFILE_DIR)/docs/*.md
	$(call footer, $@)

clean:
	$(call title, $@)
	find . -name \*~ -delete
	find . -name \#\* -delete
	rm -rf $(CURDIR)/_output/* $(MAKEFILE_DIR)/vendor
	$(call footer, $@)

##########################
# Linting tasks
##########################
lint-go:
	$(call title, $@: $(GOOS))
	@cd $(MAKEFILE_DIR) \
		&& golangci-lint run $(VERBOSE_FLAG_LONG) ./...
	$(call footer, $@)

lint-go-all:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& GOOS=linux make lint-go \
		&& GOOS=windows make lint-go \
		&& GOOS=freebsd make lint-go
	$(call footer, $@)

lint-imports:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& goimports-reviser -recursive -list-diff -set-exit-status -output stdout -company-prefixes "$(ORG_PREFIXES)"  ./...
	$(call footer, $@)

lint-yaml:
	$(call title, $@)
	cd $(MAKEFILE_DIR) \
		&& yamllint .
	$(call footer, $@)

lint-shell: $(call recursive_wildcard,$(MAKEFILE_DIR)/,*.sh)
	$(call title, $@)
	shellcheck -a -x $^
	$(call footer, $@)

lint-commits:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& git-validation $(VERBOSE_FLAG) -run DCO,short-subject,dangling-whitespace -range "$(LINT_COMMIT_RANGE)"
	$(call footer, $@)

lint-mod:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& go mod tidy --diff
	$(call footer, $@)

# FIXME: go-licenses cannot find LICENSE from root of repo when submodule is imported:
# https://github.com/google/go-licenses/issues/186
# This is impacting gotest.tools
# FIXME: go-base36 is multi-license (MIT/Apache), using a custom boilerplate file that go-licenses fails to understand
lint-licenses:
	$(call title, $@: $(GOOS))
	@cd $(MAKEFILE_DIR) \
		&& go-licenses check --include_tests --allowed_licenses=Apache-2.0,BSD-2-Clause,BSD-2-Clause-FreeBSD,BSD-3-Clause,MIT,ISC,Python-2.0,PostgreSQL,X11,Zlib \
		  --ignore gotest.tools \
		  --ignore github.com/multiformats/go-base36 \
		  ./...
	$(call footer, $@)

lint-licenses-all:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& GOOS=linux make lint-licenses \
		&& GOOS=freebsd make lint-licenses \
		&& GOOS=windows make lint-licenses
	$(call footer, $@)

##########################
# Automated fixing tasks
##########################
fix-go:
	$(call title, $@: $(GOOS))
	@cd $(MAKEFILE_DIR) \
		&& golangci-lint run --fix
	$(call footer, $@)

fix-go-all:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& GOOS=linux make fix-go \
		&& GOOS=freebsd make fix-go \
		&& GOOS=windows make fix-go
	$(call footer, $@)

fix-imports:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& goimports-reviser -company-prefixes $(ORG_PREFIXES) ./...
	$(call footer, $@)

fix-mod:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& go mod tidy
	$(call footer, $@)

##########################
# Development tools installation
##########################
install-dev-tools:
	$(call title, $@)
	# golangci: v1.64.5
	# git-validation: main from 2023/11
	# ltag: v0.2.5
	# go-licenses: v2.0.0-alpha.1
	# goimports-reviser: v3.8.2
	@cd $(MAKEFILE_DIR) \
		&& go install github.com/golangci/golangci-lint/cmd/golangci-lint@0a603e49e5e9870f5f9f2035bcbe42cd9620a9d5 \
		&& go install github.com/vbatts/git-validation@679e5cad8c50f1605ab3d8a0a947aaf72fb24c07 \
		&& go install github.com/kunalkushwaha/ltag@b0cfa33e4cc9383095dc584d3990b62c95096de0 \
		&& go install github.com/google/go-licenses/v2@d01822334fba5896920a060f762ea7ecdbd086e8 \
		&& go install github.com/incu6us/goimports-reviser/v3@f034195cc8a7ffc7cc70d60aa3a25500874eaf04 \
		&& go install gotest.tools/gotestsum@ac6dad9c7d87b969004f7749d1942938526c9716
	@echo "Remember to add \$$HOME/go/bin to your path"
	$(call footer, $@)

GO_VERSION ?= stable
GO_VERSION_SELECTOR =
GO_VERSION_SELECTOR = .version | startswith("go$(GO_VERSION)")
ifeq ($(GO_VERSION),canary)
	GO_VERSION_SELECTOR = .stable==false
endif
ifeq ($(GO_VERSION),stable)
	GO_VERSION_SELECTOR = .stable==true
endif
ifeq ($(GO_VERSION),)
	GO_VERSION_SELECTOR = .stable==true
endif

GO_INSTALL_DESTINATION ?= /opt/$(BINARY)-dev-tools

install-go:
	$(call title, $@)
	@mkdir -p $(GO_INSTALL_DESTINATION)
	@if [ ! -e $(GO_INSTALL_DESTINATION)/go ]; then curl -fsSL --proto '=https' --tlsv1.2 https://go.dev/dl/$(shell curl -fsSL --proto "=https" --tlsv1.3 "https://go.dev/dl/?mode=json&include=all" | \
		jq -rc 'map(select($(GO_VERSION_SELECTOR)))[0].files | map(select(.os=="$(OS)" and .arch=="$(ARCH)"))[0].filename') | \
		tar xzC $(GO_INSTALL_DESTINATION) || exit; \
	else \
		echo "Install already detected, doing nothing."; \
	fi
	@echo Remember to add to your profile: export PATH="$(GO_INSTALL_DESTINATION)/go/bin:\$$HOME/go/bin:\$$PATH"
	$(call footer, $@)

install-go-resolve-version:
	@curl -fsSL --proto "=https" --tlsv1.2 "https://go.dev/dl/?mode=json&include=all" | \
		jq -rc 'map(select($(GO_VERSION_SELECTOR)))[0].version' | sed s/go//

##########################
# Testing tasks
##########################
test-unit:
	$(call title, $@)
	@go test $(VERBOSE_FLAG) $(MAKEFILE_DIR)/pkg/...
	$(call footer, $@)

test-unit-bench:
	$(call title, $@)
	@go test $(VERBOSE_FLAG) $(MAKEFILE_DIR)/pkg/... -bench=.
	$(call footer, $@)

test-unit-race:
	$(call title, $@)
	@go test $(VERBOSE_FLAG) $(MAKEFILE_DIR)/pkg/... -race
	$(call footer, $@)

##########################
# Release tasks
##########################
# Note that these options will not work on macOS - unless you use gnu-tar instead of tar
TAR_OWNER0_FLAGS=--owner=0 --group=0
TAR_FLATTEN_FLAGS=--transform 's/.*\///g'

define make_artifact_full_linux
	$(DOCKER) build --output type=tar,dest=$(CURDIR)/_output/nerdctl-full-$(VERSION_TRIMMED)-linux-$(1).tar --target out-full --platform $(1) --build-arg GO_VERSION -f $(MAKEFILE_DIR)/Dockerfile $(MAKEFILE_DIR)
	gzip -9 $(CURDIR)/_output/nerdctl-full-$(VERSION_TRIMMED)-linux-$(1).tar
endef

artifacts: clean
	$(call title, $@)
	GOOS=linux GOARCH=amd64       make -C $(CURDIR) -f $(MAKEFILE_DIR)/Makefile binaries
	tar $(TAR_OWNER0_FLAGS) $(TAR_FLATTEN_FLAGS) -czvf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-linux-amd64.tar.gz   $(CURDIR)/_output/nerdctl $(MAKEFILE_DIR)/extras/rootless/*

	GOOS=linux GOARCH=arm64       make -C $(CURDIR) -f $(MAKEFILE_DIR)/Makefile binaries
	tar $(TAR_OWNER0_FLAGS) $(TAR_FLATTEN_FLAGS) -czvf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-linux-arm64.tar.gz   $(CURDIR)/_output/nerdctl $(MAKEFILE_DIR)/extras/rootless/*

	GOOS=linux GOARCH=arm GOARM=7 make -C $(CURDIR) -f $(MAKEFILE_DIR)/Makefile binaries
	tar $(TAR_OWNER0_FLAGS) $(TAR_FLATTEN_FLAGS) -czvf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-linux-arm-v7.tar.gz  $(CURDIR)/_output/nerdctl $(MAKEFILE_DIR)/extras/rootless/*

	GOOS=linux GOARCH=ppc64le     make -C $(CURDIR) -f $(MAKEFILE_DIR)/Makefile binaries
	tar $(TAR_OWNER0_FLAGS) $(TAR_FLATTEN_FLAGS) -czvf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-linux-ppc64le.tar.gz $(CURDIR)/_output/nerdctl $(MAKEFILE_DIR)/extras/rootless/*

	GOOS=linux GOARCH=riscv64     make -C $(CURDIR) -f $(MAKEFILE_DIR)/Makefile binaries
	tar $(TAR_OWNER0_FLAGS) $(TAR_FLATTEN_FLAGS) -czvf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-linux-riscv64.tar.gz $(CURDIR)/_output/nerdctl $(MAKEFILE_DIR)/extras/rootless/*

	GOOS=linux GOARCH=s390x       make -C $(CURDIR) -f $(MAKEFILE_DIR)/Makefile binaries
	tar $(TAR_OWNER0_FLAGS) $(TAR_FLATTEN_FLAGS) -czvf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-linux-s390x.tar.gz   $(CURDIR)/_output/nerdctl $(MAKEFILE_DIR)/extras/rootless/*

	GOOS=windows GOARCH=amd64     make -C $(CURDIR) -f $(MAKEFILE_DIR)/Makefile binaries
	tar $(TAR_OWNER0_FLAGS) $(TAR_FLATTEN_FLAGS) -czvf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-windows-amd64.tar.gz $(CURDIR)/_output/nerdctl.exe

	GOOS=freebsd GOARCH=amd64     make -C $(CURDIR) -f $(MAKEFILE_DIR)/Makefile binaries
	tar $(TAR_OWNER0_FLAGS) $(TAR_FLATTEN_FLAGS) -czvf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-freebsd-amd64.tar.gz $(CURDIR)/_output/nerdctl

	rm -f $(CURDIR)/_output/nerdctl $(CURDIR)/_output/nerdctl.exe

	$(call make_artifact_full_linux,amd64)
	$(call make_artifact_full_linux,arm64)

	$(GO) -C $(MAKEFILE_DIR) mod vendor
	tar $(TAR_OWNER0_FLAGS) -czf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-go-mod-vendor.tar.gz $(MAKEFILE_DIR)/go.mod $(MAKEFILE_DIR)/go.sum $(MAKEFILE_DIR)/vendor
	$(call footer, $@)

.PHONY: \
	all \
	lint \
	fix \
	test \
	help \
	binaries \
	install \
	clean \
	lint-go lint-go-all lint-imports lint-yaml lint-shell lint-commits lint-mod lint-licenses lint-licenses-all \
	fix-go fix-go-all fix-imports fix-mod \
	install-dev-tools \
	test-unit test-unit-race test-unit-bench \
	artifacts
