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

# Classifies the tests of a gotestsum json file, and reports them to the flaky test dashboard.
# This is the *emitting* half of the dashboard: call it at the end of a test job
# (`gotestsum --post-run-command`, or a plain step). Two classes are reported:
# - "failing": the tests that failed and never passed, even on retry.
# - "flaky": the tests that failed, then passed on retry. Flaky beyond a doubt. Those only exist
#   when the tests were retried, which gotestsum does with `--rerun-fails`.
#
# Each class is reported twice, for two different readers:
# - as one marker line per test in the job log (`flaky-test-dashboard: failing TestFoo`), which is
#   what flaky-report.sh collects, from the archived logs of the workflow run;
# - as a GitHub Actions annotation, which is what a human sees on a pull request.
# Both are defined in lib.sh, and are a contract with the collector.
#
# Usage:
#   gotestsum --jsonfile=/tmp/test.log --rerun-fails=2 --post-run-command "flaky-annotate.sh" ...
#   flaky-annotate.sh [gotestsum-jsonfile]     # defaults to $GOTESTSUM_JSONFILE
#
# Environment:
#   SOIGNEUR_FAILING_OUT  if set, the list of consistently failing tests is written to that file
#   SOIGNEUR_SOIGNEUR_OUT if set, the list of tests that recovered on retry is written to that file
#                         (both are meant for callers that render their own step summary)
#   SOIGNEUR_QUIET        if true, only the annotations are emitted, not the readable blocks

set -o errexit -o errtrace -o functrace -o nounset -o pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]:-$PWD}")" 2>/dev/null 1>&2 && pwd)"
readonly root

# shellcheck source-path=SCRIPTDIR
. "$root"/lib.sh

: "${SOIGNEUR_FAILING_OUT:=}"
: "${SOIGNEUR_SOIGNEUR_OUT:=}"
: "${SOIGNEUR_QUIET:=false}"

# Prints a list where a human will look for it: at the end of the job log, past the noise of the
# run itself. The block is also the format that flaky-report.sh falls back to for the logs that
# predate the markers, hence the closing delimiter, which is what tells it where the list ends.
soigneur::block(){
  local title="$1"
  local list="$2"
  local rule

  rule="$(printf '%*s' "$((${#title} + 8))" '')"
  printf '\n=== %s ===\n%s\n%s\n' "$title" "$list" "${rule// /=}"
}

# Writes one marker line per test, inside a collapsed group so that the job log stays readable.
soigneur::mark(){
  local kind="$1"
  local list="$2"

  echo "::group::$soigneur_marker $kind"
  while read -r test; do
    [ "$test" == "" ] || printf "%s %s %s\n" "$soigneur_marker" "$kind" "$test"
  done <<< "$list"
  echo "::endgroup::"
}

# Emits a GitHub Actions annotation. Multi-line messages need their newlines as %0A.
soigneur::annotate(){
  local level="$1"
  local title="$2"
  local msg="$3"

  echo "::$level title=$title::${msg//$'\n'/%0A}"
}

# Prints the tests of one class, using the outcomes of the whole run: a test that failed and then
# passed was retried and recovered ("flaky"); one that only ever failed is "failing".
soigneur::classify(){
  local want="$1"
  local outcomes="$2"

  printf "%s\n" "$outcomes" | awk -F'\t' -v want="$want" '
    $1 == "fail" { failed[$2] = 1 }
    $1 == "pass" { passed[$2] = 1 }
    END {
      for (t in failed) {
        if (((t in passed) ? "flaky" : "failing") == want) print t
      }
    }
  ' | sort
}

soigneur::main(){
  local jsonfile="${1:-${GOTESTSUM_JSONFILE:-}}"
  local outcomes failing flaky

  [ "$jsonfile" != "" ] || {
    echo "usage: $0 <gotestsum-jsonfile>" >&2
    return 1
  }
  [ -r "$jsonfile" ] || {
    echo "error: cannot read $jsonfile" >&2
    return 1
  }

  outcomes="$(jq -rc 'select(.Test) | select(.Action == "fail" or .Action == "pass") | [.Action, .Test] | @tsv' < "$jsonfile")"
  failing="$(soigneur::classify failing "$outcomes")"
  flaky="$(soigneur::classify flaky "$outcomes")"

  [ "$SOIGNEUR_FAILING_OUT" == "" ] || printf "%s\n" "$failing" > "$SOIGNEUR_FAILING_OUT"
  [ "$SOIGNEUR_SOIGNEUR_OUT" == "" ] || printf "%s\n" "$flaky" > "$SOIGNEUR_SOIGNEUR_OUT"

  if [ "$failing" != "" ]; then
    soigneur::bool "$SOIGNEUR_QUIET" || soigneur::block "$soigneur_title_failing" "$failing"
    soigneur::mark "failing" "$failing"
    soigneur::annotate "error" "$soigneur_title_failing" "$failing"
  fi

  if [ "$flaky" != "" ]; then
    soigneur::bool "$SOIGNEUR_QUIET" || soigneur::block "$soigneur_title_flaky (passed on retry)" "$flaky"
    soigneur::mark "flaky" "$flaky"
    soigneur::annotate "warning" "$soigneur_title_flaky" "$flaky"
  fi
}

soigneur::main "$@"
