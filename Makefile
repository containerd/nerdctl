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

GO ?= go
GOOS ?= $(shell $(GO) env GOOS)
ifeq ($(GOOS),windows)
	BIN_EXT := .exe
endif

PACKAGE := github.com/containerd/nerdctl

# distro builders might wanna override these
PREFIX  ?= /usr/local
BINDIR  ?= $(PREFIX)/bin
DATADIR ?= $(PREFIX)/share
DOCDIR  ?= $(DATADIR)/doc

VERSION ?= $(shell git describe --match 'v[0-9]*' --dirty='.m' --always --tags)
VERSION_TRIMMED := $(VERSION:v%=%)
REVISION ?= $(shell git rev-parse HEAD)$(shell if ! git diff --no-ext-diff --quiet --exit-code; then echo .m; fi)

GO_BUILD_LDFLAGS ?= -s -w
GO_BUILD_FLAGS ?=
export GO_BUILD=GO111MODULE=on CGO_ENABLED=0 GOOS=$(GOOS) $(GO) build -ldflags "$(GO_BUILD_LDFLAGS) -X $(PACKAGE)/pkg/version.Version=$(VERSION) -X $(PACKAGE)/pkg/version.Revision=$(REVISION)"

ifdef VERBOSE
	VERBOSE_FLAG := -v
endif

all: binaries

help:
	@echo "Usage: make <target>"
	@echo
	@echo " * 'install' - Install binaries to system locations."
	@echo " * 'binaries' - Build nerdctl."
	@echo " * 'clean' - Clean artifacts."

nerdctl:
	$(GO_BUILD) $(GO_BUILD_FLAGS) $(VERBOSE_FLAG) -o $(CURDIR)/_output/nerdctl$(BIN_EXT) $(PACKAGE)/cmd/nerdctl

clean:
	find . -name \*~ -delete
	find . -name \#\* -delete
	rm -rf _output/* vendor

binaries: nerdctl

install:
	install -D -m 755 $(CURDIR)/_output/nerdctl $(DESTDIR)$(BINDIR)/nerdctl
	install -D -m 755 $(CURDIR)/extras/rootless/containerd-rootless.sh $(DESTDIR)$(BINDIR)/containerd-rootless.sh
	install -D -m 755 $(CURDIR)/extras/rootless/containerd-rootless-setuptool.sh $(DESTDIR)$(BINDIR)/containerd-rootless-setuptool.sh
	install -D -m 644 -t $(DESTDIR)$(DOCDIR)/nerdctl docs/*.md

define make_artifact_full_linux
	DOCKER_BUILDKIT=1 docker build --output type=tar,dest=$(CURDIR)/_output/nerdctl-full-$(VERSION_TRIMMED)-linux-$(1).tar --target out-full --platform $(1) --build-arg GO_VERSION $(CURDIR)
	gzip -9 $(CURDIR)/_output/nerdctl-full-$(VERSION_TRIMMED)-linux-$(1).tar
endef

TAR_OWNER0_FLAGS=--owner=0 --group=0
TAR_FLATTEN_FLAGS=--transform 's/.*\///g'

artifacts: clean
	GOOS=linux GOARCH=amd64       make -C $(CURDIR)  binaries
	tar $(TAR_OWNER0_FLAGS) $(TAR_FLATTEN_FLAGS) -czvf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-linux-amd64.tar.gz   _output/nerdctl extras/rootless/*

	GOOS=linux GOARCH=arm64       make -C $(CURDIR) binaries
	tar $(TAR_OWNER0_FLAGS) $(TAR_FLATTEN_FLAGS) -czvf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-linux-arm64.tar.gz   _output/nerdctl extras/rootless/*

	GOOS=linux GOARCH=arm GOARM=7 make -C $(CURDIR) binaries
	tar $(TAR_OWNER0_FLAGS) $(TAR_FLATTEN_FLAGS) -czvf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-linux-arm-v7.tar.gz  _output/nerdctl extras/rootless/*

	GOOS=linux GOARCH=ppc64le     make -C $(CURDIR) binaries
	tar $(TAR_OWNER0_FLAGS) $(TAR_FLATTEN_FLAGS) -czvf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-linux-ppc64le.tar.gz _output/nerdctl extras/rootless/*

	GOOS=linux GOARCH=riscv64     make -C $(CURDIR) binaries
	tar $(TAR_OWNER0_FLAGS) $(TAR_FLATTEN_FLAGS) -czvf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-linux-riscv64.tar.gz   _output/nerdctl extras/rootless/*

	GOOS=linux GOARCH=s390x       make -C $(CURDIR) binaries
	tar $(TAR_OWNER0_FLAGS) $(TAR_FLATTEN_FLAGS) -czvf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-linux-s390x.tar.gz   _output/nerdctl extras/rootless/*

	GOOS=windows GOARCH=amd64     make -C $(CURDIR) binaries
	tar $(TAR_OWNER0_FLAGS) $(TAR_FLATTEN_FLAGS) -czvf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-windows-amd64.tar.gz _output/nerdctl.exe

	GOOS=freebsd GOARCH=amd64     make -C $(CURDIR)  binaries
	tar $(TAR_OWNER0_FLAGS) $(TAR_FLATTEN_FLAGS) -czvf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-freebsd-amd64.tar.gz _output/nerdctl

	rm -f $(CURDIR)/_output/nerdctl $(CURDIR)/_output/nerdctl.exe

	$(call make_artifact_full_linux,amd64)
	$(call make_artifact_full_linux,arm64)

	go mod vendor
	tar $(TAR_OWNER0_FLAGS) -czf $(CURDIR)/_output/nerdctl-$(VERSION_TRIMMED)-go-mod-vendor.tar.gz go.mod go.sum vendor

.PHONY: \
	help \
	nerdctl \
	clean \
	binaries \
	install \
	artifacts
