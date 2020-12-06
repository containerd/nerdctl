# Copyright (C) The Kubernetes Authors.
# Copyright (C) containerd Authors.
# Copyright (C) nerdctl Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

GO ?= go

export GO_BUILD=$(GO) build
export GO_TEST=$(GO) test

PROJECT := github.com/AkihiroSuda/nerdctl
BINDIR ?= /usr/local/bin

VERSION := $(shell git describe --tags --dirty --always)
VERSION := $(VERSION:v%=%)

BUILD_BIN_PATH := $(shell pwd)/build/bin

GOLANGCI_LINT := $(BUILD_BIN_PATH)/golangci-lint
GOLANGCI_LINT_VERSION := "v1.32.2"

all: binaries

help:
	@echo "Usage: make <target>"
	@echo
	@echo " * 'install' - Install binaries to system locations."
	@echo " * 'binaries' - Build nerdctl."
	@echo " * 'clean' - Clean artifacts."

nerdctl:
	CGO_ENABLED=0 $(GO_BUILD) -o $(CURDIR)/_output/nerdctl \
		-ldflags '$(GO_LDFLAGS)' \
		-tags '$(BUILDTAGS)' \
		$(PROJECT)

clean:
	find . -name \*~ -delete
	find . -name \#\* -delete
	rm -rf _output/*

binaries: nerdctl

build: nerdctl

install-nerdctl:
	install -D -m 755 $(CURDIR)/_output/nerdctl $(DESTDIR)$(BINDIR)/nerdctl

install: install-nerdctl

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run

install.tools: install.lint

install.lint: $(GOLANGCI_LINT)

$(GOLANGCI_LINT):
	export \
		VERSION=$(GOLANGCI_LINT_VERSION) \
		URL=https://raw.githubusercontent.com/golangci/golangci-lint \
		BINDIR=${BUILD_BIN_PATH} && \
	curl -sfL $$URL/$$VERSION/install.sh | sh -s $$VERSION

.PHONY: \
	help \
	nerdctl \
	clean \
	binaries \
	install \
	install-nerdctl \
	lint \
	install.tools
