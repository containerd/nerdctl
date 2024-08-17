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

## This is a library of generic helpers that can be used across different projects

# Simple logger
readonly LOG_LEVEL_DEBUG=0
readonly LOG_LEVEL_INFO=1
readonly LOG_LEVEL_WARNING=2
readonly LOG_LEVEL_ERROR=3

readonly LOG_COLOR_BLACK=0
readonly LOG_COLOR_RED=1
readonly LOG_COLOR_GREEN=2
readonly LOG_COLOR_YELLOW=3
readonly LOG_COLOR_BLUE=4
readonly LOG_COLOR_MAGENTA=5
readonly LOG_COLOR_CYAN=6
readonly LOG_COLOR_WHITE=7
readonly LOG_COLOR_DEFAULT=9

readonly LOG_STYLE_DEBUG=( setaf "$LOG_COLOR_WHITE" )
readonly LOG_STYLE_INFO=( setaf "$LOG_COLOR_GREEN" )
readonly LOG_STYLE_WARNING=( setaf "$LOG_COLOR_YELLOW" )
readonly LOG_STYLE_ERROR=( setaf "$LOG_COLOR_RED" )

_log::log(){
  local level
  local style
  local numeric_level
  local message="$2"

  level="$(printf "%s" "$1" | tr '[:lower:]' '[:upper:]')"
  numeric_level="$(printf "LOG_LEVEL_%s" "$level")"
  style="LOG_STYLE_${level}[@]"

  [ "${!numeric_level}" -ge "$LOG_LEVEL" ] || return 0

  [ ! "$TERM" ] || [ ! -t 2 ] || >&2 tput "${!style}" 2>/dev/null || true
  >&2 printf "[%s] %s: %s\n" "$(date 2>/dev/null || true)" "$(printf "%s" "$level" | tr '[:lower:]' '[:upper:]')" "$message"
  [ ! "$TERM" ] || [ ! -t 2 ] || >&2 tput op 2>/dev/null || true
}

log::init(){
  local _ll
  # Default log to warning if unspecified
  _ll="$(printf "LOG_LEVEL_%s" "${LOG_LEVEL:-warning}" | tr '[:lower:]' '[:upper:]')"
  # Default to 3 (warning) if unrecognized
  LOG_LEVEL="${!_ll:-3}"
}

log::debug(){
  _log::log debug "$@"
}

log::info(){
  _log::log info "$@"
}

log::warning(){
  _log::log warning "$@"
}

log::error(){
  _log::log error "$@"
}

# Helpers
host::require(){
  local binary="$1"

  log::debug "Checking presence of $binary"
  command -v "$binary" >/dev/null || {
    log::error "You need $binary for this script to work, and it cannot be found in your path"
    return 1
  }
}

host::install(){
  local binary

  for binary in "$@"; do
    log::debug "sudo install -D -m 755 $binary /usr/local/bin/$(basename "$binary")"
    sudo install -D -m 755 "$binary" /usr/local/bin/"$(basename "$binary")"
  done
}

fs::mktemp(){
  local prefix="${1:-temporary}"

  mktemp -dq "${TMPDIR:-/tmp}/$prefix.XXXXXX" 2>/dev/null || mktemp -dq || {
    log::error "Failed to create temporary directory"
    return 1
  }
}

tar::expand(){
  local dir="$1"
  local arc="$2"

  log::debug "tar -C $dir -xzf $arc"
  tar -C "$dir" -xzf "$arc"
}

_http::get(){
  local url="$1"
  local output="$2"
  local retry="$3"
  local delay="$4"
  local user="${5:-}"
  local password="${6:-}"
  shift
  shift
  shift
  shift
  shift
  shift

  local header
  local command=(curl -fsSL --retry "$retry" --retry-delay "$delay" -o "$output")
  # Add a basic auth user if necessary
  [ "$user" == "" ] || command+=(--user "$user:$password")
  # Force tls v1.2 and no redirect to http if url scheme is https
  [ "${url:0:5}" != "https" ] || command+=(--proto '=https' --tlsv1.2)
  # Stuff in any additional arguments as headers
  for header in "$@"; do
    command+=(-H "$header")
  done
  # Debug
  log::debug "${command[*]} $url"
  # Exec
  "${command[@]}" "$url" || {
    log::error "Failed to connect to $url with $retry retries every $delay seconds"
    return 1
  }
}

http::get(){
  local output="$1"
  local url="$2"
  shift
  shift

  _http::get "$url" "$output" "2" "1" "" "" "$@"
}

http::healthcheck(){
  local url="$1"
  local retry="${2:-5}"
  local delay="${3:-1}"
  local user="${4:-}"
  local password="${5:-}"
  shift
  shift
  shift
  shift
  shift

  _http::get "$url" /dev/null "$retry" "$delay" "$user" "$password" "$@"
}

http::checksum(){
  local urls=("$@")
  local url

  local temp
  temp="$(fs::mktemp "http-checksum")"

  host::require shasum

  for url in "${urls[@]}"; do
    http::get "$temp/${url##*/}" "$url"
  done

  cd "$temp"
  shasum -a 256 ./*
  cd - >/dev/null || true
}

# Github API helpers
# Set GITHUB_TOKEN to use authenticated requests to workaround limitations

github::settoken(){
  local token="$1"
  # If passed token is a github action pattern replace, and we are NOT on github, ignore it
  # shellcheck disable=SC2016
  [ "${token:0:3}" == '${{' ] || GITHUB_TOKEN="$token"
}

github::request(){
  local endpoint="$1"
  local args=(
    "Accept: application/vnd.github+json"
    "X-GitHub-Api-Version: 2022-11-28"
  )

  [ "${GITHUB_TOKEN:-}" == "" ] || args+=("Authorization: Bearer $GITHUB_TOKEN")

  http::get /dev/stdout https://api.github.com/"$endpoint" "${args[@]}"
}

github::tags::latest(){
  local repo="$1"
  github::request "repos/$repo/tags" | jq -rc .[0].name
}

github::releases(){
  local repo="$1"
  github::request "repos/$repo/releases" |
    jq -rc .[]
}

github::releases::latest(){
  local repo="$1"
  github::request "repos/$repo/releases/latest" | jq -rc .
}

log::init
host::require jq
host::require tar
host::require curl
