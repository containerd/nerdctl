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


PACKAGE := github.com/AkihiroSuda/nerdctl
BINDIR ?= /usr/local/bin

VERSION=$(shell git describe --match 'v[0-9]*' --dirty='.m' --always --tags)
REVISION=$(shell git rev-parse HEAD)$(shell if ! git diff --no-ext-diff --quiet --exit-code; then echo .m; fi)

export GO_BUILD=GO111MODULE=on CGO_ENABLED=0 $(GO) build -ldflags "-X $(PACKAGE)/pkg/version.Version=$(VERSION) -X $(PACKAGE)/pkg/version.Revision=$(REVISION)"

all: binaries

help:
	@echo "Usage: make <target>"
	@echo
	@echo " * 'install' - Install binaries to system locations."
	@echo " * 'binaries' - Build nerdctl."
	@echo " * 'clean' - Clean artifacts."

nerdctl:
	$(GO_BUILD) -o $(CURDIR)/_output/nerdctl $(PACKAGE)

clean:
	find . -name \*~ -delete
	find . -name \#\* -delete
	rm -rf _output/*

binaries: nerdctl

install:
	install -D -m 755 $(CURDIR)/_output/nerdctl $(DESTDIR)$(BINDIR)/nerdctl

artifacts:
	rm -f $(CURDIR)/_output/nerdctl
	GOOS=linux GOARCH=amd64       $(GO_BUILD) -o $(CURDIR)/_output/nerdctl-$(VERSION)-linux-amd64 $(PACKAGE)
	GOOS=linux GOARCH=arm64       $(GO_BUILD) -o $(CURDIR)/_output/nerdctl-$(VERSION)-linux-arm64 $(PACKAGE)
	GOOS=linux GOARCH=arm GOARM=7 $(GO_BUILD) -o $(CURDIR)/_output/nerdctl-$(VERSION)-linux-arm-v7 $(PACKAGE)

.PHONY: \
	help \
	nerdctl \
	clean \
	binaries \
	install \
	artifacts
