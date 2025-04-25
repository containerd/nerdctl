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

set -o errexit -o errtrace -o functrace -o nounset -o pipefail

# go::canary::for::go-setup retrieves the latest unstable golang version and format it for use by
# https://github.com/actions/setup-go
# Note that if the latest unstable is an old RC for the current stable, we print the empty string.
go::canary::for::go-setup(){
  local all_versions
  local stable_major_minor
  local canary_major_minor
  local canary_full_version

  # Get all golang versions
  all_versions="$(curl -fsSL --proto '=https' --tlsv1.2 "https://go.dev/dl/?mode=json&include=all")"

  # Get the latest stable release major.minor for comparison
  read -r stable_major_minor < \
    <(sed -E 's/^go([0-9]+[.][0-9]+).*/\1/i' \
      <(jq -rc 'map(select(.stable==true)).[0].version' <<<"$all_versions") \
    )

  # Get the latest unstable release major.minor, and full version (formatted for use by go-setup)
  read -r canary_major_minor canary_full_version < \
    <(sed -E 's/^go([0-9]+)[.]([0-9]+)(([a-z]+)([0-9]+))?/\1.\2 \1.\2.0-\4.\5/i' \
      <(jq -rc 'map(select(.stable==false)).[0].version' <<<"$all_versions") \
    )

  # If the latest RC is for the same major.minor as the latest stable one, then there is no canary, return empty string
  [ "$canary_major_minor" != "$stable_major_minor" ] || return 0

  # Otherwise, print the full version
  printf "%s" "$canary_full_version"
}

# github::project::latest retrieves the latest tag from a github project
github::project::latest(){
  local project="$1"
  local args

  # Get latest
  args=(curl -fsSL --proto '=https' --tlsv1.2 -H "Accept: application/vnd.github+json" -H "X-GitHub-Api-Version: 2022-11-28")
  [ "${GITHUB_TOKEN:-}" == "" ] && {
    >&2 printf "GITHUB_TOKEN is not set - you might face rate limitations with the Github API\n"
  } || args+=(-H "Authorization: Bearer $GITHUB_TOKEN")

  "${args[@]}" https://api.github.com/repos/"$project"/tags | jq -rc .[0].name
}