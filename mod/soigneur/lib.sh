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

# Definitions shared by the two halves of the flaky test dashboard.

# shellcheck disable=SC2034
{
  # The marker that flaky-annotate.sh writes to the job log, and that flaky-report.sh reads back
  # from the archived logs of a workflow run. This is the contract between the two halves:
  #
  #   flaky-test-dashboard: failing TestFoo
  #   flaky-test-dashboard: flaky TestBar/subtest
  #
  # One line per test, so that nothing depends on the layout of the log around it. Changing this
  # breaks the collection of every job that still runs the previous version, hence the constant.
  readonly soigneur_marker="flaky-test-dashboard:"

  # The titles of the annotations that flaky-annotate.sh emits for the GitHub web UI. They carry
  # the same information as the markers, for humans looking at a pull request.
  readonly soigneur_title_failing="Failing tests"
  readonly soigneur_title_flaky="Flaky tests"
}

# Fails, with a message naming the variable, when a value is not a plain non-negative integer.
# The knobs reach the scripts as environment variables, and end up as arguments of `date`, `awk`,
# `jq`, and `gh`: a value that is not a number deserves to be named, and one that starts with a
# dash would be read as a flag by whatever it is passed to.
soigneur::number(){
  local name="$1"
  local value="$2"

  [[ "$value" =~ ^[0-9]+$ ]] || {
    echo "error: $name must be a non-negative integer, got '$value'" >&2
    return 1
  }
}

# Fails when a value is not an "owner/name" pair. It is interpolated into the API paths, and
# passed to `gh` as an argument, so it has to be neither a path traversal nor a flag.
soigneur::repo(){
  local name="$1"
  local value="$2"

  [[ "$value" =~ ^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$ ]] || {
    echo "error: $name must be OWNER/NAME, got '$value'" >&2
    return 1
  }
}

# Truthiness, the same way Go's strconv.ParseBool sees it: an explicit "false" means false, and
# so does anything unset. Never gate on non-empty.
soigneur::bool(){
  case "${1,,}" in
    1 | t | true | y | yes | on) return 0 ;;
    *) return 1 ;;
  esac
}
