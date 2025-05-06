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

# shellcheck source=/dev/null
. "$root"/action-helpers.sh

GITHUB_STEP_SUMMARY="${GITHUB_STEP_SUMMARY:-/dev/null}"

{
  github::md::h3 "Total number of tests: $TESTS_TOTAL"
  github::md::pie "Status" "Skipped" "$TESTS_SKIPPED" "Failed" "$TESTS_FAILED" "Passed" "$(( TESTS_TOTAL - TESTS_FAILED - TESTS_SKIPPED ))"

  # shellcheck disable=SC2207
  pie=($(jq -rc 'select(has("Test") | not) | select(.Elapsed) | select(.Elapsed > 0) | "\(.Package) \(.Elapsed) "' < "$GOTESTSUM_JSONFILE"))
  github::md::pie "Time spent per package" "${pie[@]}"

  github::md::h3 "Failing tests"
  echo '```'
  jq -rc 'select(.Action == "fail") | select(.Test) | .Test' < "$GOTESTSUM_JSONFILE"
  echo '```'

  github::md::h3 "Tests taking more than 15 seconds"
  echo '```'
  gotestsum tool slowest --threshold 15s --jsonfile "$GOTESTSUM_JSONFILE"
  echo '```'
} >> "$GITHUB_STEP_SUMMARY"
