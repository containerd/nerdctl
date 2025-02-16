#!/usr/bin/env bash

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

# shellcheck disable=SC2034,SC2015
set -o errexit -o errtrace -o functrace -o nounset -o pipefail

# FIXME: go-licenses cannot find LICENSE from root of repo when submodule is imported:
# https://github.com/google/go-licenses/issues/186
# This is impacting gotest.tools
# go-licenses is also really broken right now wrt to stdlib: https://github.com/google/go-licenses/issues/244
# workaround taken from the awesome folks at Pulumi: https://github.com/pulumi/license-check-action/pull/3
go-licenses check --include_tests --allowed_licenses=Apache-2.0,BSD-2-Clause,BSD-3-Clause,MIT,MPL-2.0 \
	  --ignore gotest.tools \
	  --ignore "$(go list std | awk 'NR > 1 { printf(",") } { printf("%s",$0) } END { print "" }')" \
	  ./...

printf "WARNING: you need to manually verify licenses for:\n- gotest.tools\n"
