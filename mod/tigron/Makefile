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

# project-checks is broken.
# See https://github.com/containerd/nerdctl/pull/3889

##########################
# Configuration
##########################
ORG_PREFIXES := "github.com/containerd"

MAKEFILE_DIR := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
VERSION ?= $(shell git -C $(MAKEFILE_DIR) describe --match 'v[0-9]*' --dirty='.m' --always --tags)
VERSION_TRIMMED := $(VERSION:v%=%)
REVISION ?= $(shell git -C $(MAKEFILE_DIR) rev-parse HEAD)$(shell if ! git -C $(MAKEFILE_DIR) diff --no-ext-diff --quiet --exit-code; then echo .m; fi)
LINT_COMMIT_RANGE ?= main..HEAD

##########################
# Helpers
##########################
ifdef VERBOSE
	VERBOSE_FLAG := -v
	VERBOSE_FLAG_LONG := --verbose
endif

ifndef NO_COLORS
    NC := \033[0m
    GREEN := \033[1;32m
    ORANGE := \033[1;33m
endif

# Helpers
recursive_wildcard=$(wildcard $1$2) $(foreach e,$(wildcard $1*),$(call recursive_wildcard,$e/,$2))

define title
	@printf "$(GREEN)____________________________________________________________________________________________________\n"
	@printf "$(GREEN)%*s\n" $$(( ( $(shell echo "🐯$(1) 🐯" | wc -c ) + 100 ) / 2 )) "🐯$(1) 🐯"
	@printf "$(GREEN)____________________________________________________________________________________________________\n$(ORANGE)"
endef

define footer
	@printf "$(GREEN)> %s: done!\n" "$(1)"
	@printf "$(GREEN)____________________________________________________________________________________________________\n$(NC)"
endef

##########################
# High-level tasks definitions
##########################
lint: lint-go-all lint-imports lint-yaml lint-shell lint-commits lint-headers lint-mod lint-licenses-all
test: test-unit test-unit-race test-unit-bench
unit: test-unit test-unit-race test-unit-bench
fix: fix-mod fix-imports fix-go-all

##########################
# Linting tasks
##########################
lint-go:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& golangci-lint run $(VERBOSE_FLAG_LONG) ./...
	$(call footer, $@)

lint-go-all:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& GOOS=darwin make lint-go \
		&& GOOS=linux make lint-go \
		&& GOOS=windows make lint-go
	$(call footer, $@)

lint-imports:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& goimports-reviser -recursive -list-diff -set-exit-status -output stdout -company-prefixes "github.com/containerd"  ./...
	$(call footer, $@)

lint-yaml:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& yamllint .
	$(call footer, $@)

lint-shell: $(call recursive_wildcard,$(MAKEFILE_DIR)/,*.sh)
	$(call title, $@)
	@shellcheck -a -x $^
	$(call footer, $@)

lint-commits:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& git-validation $(VERBOSE_FLAG) -run DCO,short-subject,dangling-whitespace -range "$(LINT_COMMIT_RANGE)"
	$(call footer, $@)

lint-headers:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& ltag -t "./hack/headers" --check -v
	$(call footer, $@)

lint-mod:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& go mod tidy --diff
	$(call footer, $@)

lint-licenses:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& ./hack/make-lint-licenses.sh
	$(call footer, $@)

lint-licenses-all:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& GOOS=darwin make lint-licenses \
		&& GOOS=linux make lint-licenses \
		&& GOOS=windows make lint-licenses
	$(call footer, $@)

##########################
# Automated fixing tasks
##########################
fix-go:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& golangci-lint run --fix
	$(call footer, $@)

fix-go-all:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& GOOS=darwin make fix-go \
		&& GOOS=linux make fix-go \
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

up:
	$(call title, $@)
	@cd $(MAKEFILE_DIR) \
		&& go get -u ./...
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
	# goimports-reviser: v3.9.0
	@cd $(MAKEFILE_DIR) \
		&& go install github.com/golangci/golangci-lint/cmd/golangci-lint@0a603e49e5e9870f5f9f2035bcbe42cd9620a9d5 \
		&& go install github.com/vbatts/git-validation@679e5cad8c50f1605ab3d8a0a947aaf72fb24c07 \
		&& go install github.com/kunalkushwaha/ltag@b0cfa33e4cc9383095dc584d3990b62c95096de0 \
		&& go install github.com/google/go-licenses/v2@d01822334fba5896920a060f762ea7ecdbd086e8 \
		&& go install github.com/incu6us/goimports-reviser/v3@698f92d226d50a01731ca8551993ebc1bb7fc788
	@echo "Remember to add GOROOT/bin to your path"
	$(call footer, $@)

##########################
# Testing tasks
##########################
test-unit:
	$(call title, $@)
	@go test $(VERBOSE_FLAG) $(MAKEFILE_DIR)/...
	$(call footer, $@)

test-unit-bench:
	$(call title, $@)
	@go test $(VERBOSE_FLAG) $(MAKEFILE_DIR)/... -bench=.
	$(call footer, $@)

test-unit-race:
	$(call title, $@)
	@go test $(VERBOSE_FLAG) $(MAKEFILE_DIR)/... -race
	$(call footer, $@)

.PHONY: \
	lint \
	fix \
	test \
	up \
	unit \
	install-dev-tools \
	lint-commits lint-go lint-go-all lint-headers lint-imports lint-licenses lint-licenses-all lint-mod lint-shell lint-yaml \
	fix-go fix-go-all fix-imports fix-mod \
	test-unit test-unit-race test-unit-bench
