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

# shellcheck disable=SC2034
set -o errexit -o errtrace -o functrace -o nounset -o pipefail

readonly decorator_success="✅"
readonly decorator_failure="❌"

github::md::h1(){
  printf "# %s\n" "$1"
}

github::md::h2(){
  printf "## %s\n" "$1"
}

github::md::h3(){
  printf "### %s\n" "$1"
}

github::md::bq(){
  local x
  for x in "$@"; do
    printf "> %s\n" "$1"
  done
}

github::md::table::header(){
  printf "|"
  for x in "$@"; do
    printf " %s |" "$x"
  done
  printf "\n"
  printf "|"
  for x in "$@"; do
    printf "%s|" "---"
  done
  printf "\n"
}

github::md::table::line(){
  printf "|"
  for x in "$@"; do
    printf " %s |" "$x"
  done
  printf "\n"
}

github::md::pie(){
  local title="$1"
  local label
  local value
  shift

  printf '```mermaid\npie\n    title %s\n' "$title"
  while [ "$#" -gt 0 ]; do
    label="$1"
    value="$2"
    shift
    shift
    printf '    "%s" : %s\n' "$label" "$value"
  done
  printf '```\n\n'

}

github::log::group(){
  echo "::group::$*"
}

github::log::endgroup(){
  echo "::endgroup::"
}

github::log::warning(){
  local title="$1"
  local msg="$2"

  echo "::warning title=$title::$msg"
}

_begin=${_begin:-}
_duration=
github::timer::begin(){
  _begin="$(date +%s)"
}

github::timer::tick(){
  local tick

  tick="$(date +%s)"
  printf "%s" "$((tick - _begin))"
}

github::timer::format() {
  local t
  t="$(cat "$1")"
  local h=$((t/60/60%24))
  local m=$((t/60%60))
  local s=$((t%60))

  [[ "$h" == 0 ]] || printf "%d hours " "$h"
  [[ "$m" == 0 ]] || printf "%d minutes " "$m"
  printf '%d seconds' "$s"
}
