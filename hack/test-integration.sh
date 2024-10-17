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
root="$(cd "$(dirname "${BASH_SOURCE[0]:-$PWD}")" 2>/dev/null 1>&2 && pwd)"
readonly root
readonly timeout="60m"

# See https://github.com/containerd/nerdctl/blob/main/docs/testing/README.md#about-parallelization
args=(--format=testname --jsonfile /tmp/test-integration.log --packages="$root"/../cmd/nerdctl/...)

for arg in "$@"; do
  if [ "$arg" == "-test.only-flaky" ]; then
    args+=("--rerun-fails=2")
    break
  fi
done

gotestsum "${args[@]}" -- -timeout="$timeout" -p 1 -args -test.allow-kill-daemon "$@"

echo "These are the tests that took more than 20 seconds:"
gotestsum tool slowest --threshold 20s --jsonfile /tmp/test-integration.log
