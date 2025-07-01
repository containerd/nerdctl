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

# collector.sh provides methods to collect all system logs relevant for debugging.
# It is primarily intended to be used after a run of gotestsum has completed (successfully or not).

# shellcheck disable=SC2034,SC2015
set -o errexit -o errtrace -o functrace -o nounset -o pipefail

# collect::logs::units retrieve all running units and gather their logs
collect::logs::systemctl(){
  local destination="$1"
  local item
  local args=(--no-pager)
  if command -v systemctl >/dev/null; then
    [ "$(id -u)" == 0 ] || args+=(--user)
    for item in $(systemctl show '*' --state=running --property=Id --value "${args[@]}" | grep . | sort | uniq); do
      journalctl "${args[@]}" -u "$item" > "$destination/systemd-$item.log"
    done
  fi
}

collect::logs(){
  collect::logs::systemctl "$@"
}

collect::metadata(){
  local item
  local key
  local value
  local sep=""

  printf "{"
  for item in "$@"; do
    printf "  %s\"%s\": \"%s\"" "$sep" "${item%=*}" "${item#*=}"
    sep=","$'\n'
  done
  printf "}"
}
