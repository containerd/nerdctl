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

# The classification of the tests, and everything the flaky test dashboard needs, live in the
# reusable module: see mod/soigneur/README.md and docs/testing/flaky.md.
# - "failing": the test never passed, even on retry (either consistently broken, or not retried).
# - "flaky": the test did pass on retry, so it is flaky beyond a doubt.
# flaky-annotate.sh prints both lists to stdout, so that they stay visible at the end of the job
# log, writes the marker lines that the dashboard is collected from, and emits the matching
# annotations for the pull request. The lists come back through SOIGNEUR_FAILING_OUT and
# SOIGNEUR_SOIGNEUR_OUT, for the step summary below.
readonly soigneur="$root"/../../mod/soigneur

lists="$(mktemp -d)"
# shellcheck disable=SC2064
trap "rm -rf '$lists'" EXIT

SOIGNEUR_FAILING_OUT="$lists"/failing SOIGNEUR_SOIGNEUR_OUT="$lists"/flaky \
  "$soigneur"/flaky-annotate.sh "$GOTESTSUM_JSONFILE"

failing_tests="$(cat "$lists"/failing)"
flaky_tests="$(cat "$lists"/flaky)"

{
  github::md::h3 "Total number of tests: $TESTS_TOTAL"
  github::md::pie "Status" "Skipped" "$TESTS_SKIPPED" "Failed" "$TESTS_FAILED" "Passed" "$(( TESTS_TOTAL - TESTS_FAILED - TESTS_SKIPPED ))"

  # shellcheck disable=SC2207
  pie=($(jq -rc 'select(has("Test") | not) | select(.Elapsed) | select(.Elapsed > 0) | "\(.Package) \(.Elapsed) "' < "$GOTESTSUM_JSONFILE"))
  github::md::pie "Time spent per package" "${pie[@]}"

  github::md::h3 "Failing tests"
  echo '```'
  echo "${failing_tests:-}"
  echo '```'

  github::md::h3 "Flaky tests (failed, then passed on retry)"
  echo '```'
  echo "${flaky_tests:-}"
  echo '```'

  github::md::h3 "Tests taking more than 15 seconds"
  echo '```'
  gotestsum tool slowest --threshold 15s --jsonfile "$GOTESTSUM_JSONFILE"
  echo '```'
} >> "$GITHUB_STEP_SUMMARY"
