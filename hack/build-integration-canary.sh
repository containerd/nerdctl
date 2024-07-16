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

######################
# Definitions
######################

# "Blacklisting" here means that any dependency which name is blacklisted will be left untouched, at the version
# currently pinned in the Dockerfile.
# This is convenient so that currently broken alpha/beta/RC can be held back temporarily to keep the build green

# Currently pinned, see:
# - https://github.com/containerd/nerdctl/pull/3153
blacklist=(runc)

# List all the repositories we depend on to build and run integration tests
dependencies=(
  ktock/buildg
  moby/buildkit
  containerd/containerd
  distribution/distribution
  containers/fuse-overlayfs
  containerd/fuse-overlayfs-snapshotter
  gotestyourself/gotestsum
  ipfs/kubo
  containerd/nydus-snapshotter
  containernetworking/plugins
  rootless-containers/rootlesskit
  opencontainers/runc
  rootless-containers/slirp4netns
  awslabs/soci-snapshotter
  containerd/stargz-snapshotter
  krallin/tini
)

# Certain dependencies do issue multiple unrelated releaes on their repo - use these below to ignore certain releases
BUILDKIT_EXCLUDE="dockerfile/"
CONTAINERD_EXCLUDE="containerd API"

# Some dependencies will be checksum-matched. Setting the variables below will trigger us to download and generate shasums
# The value you set the variable to also decides which artifacts you are interested in.
BUILDKIT_CHECKSUM=linux
CNI_PLUGINS_CHECKSUM=linux
CONTAINERD_FUSE_OVERLAYFS_CHECKSUM=linux
FUSE_OVERLAYFS_CHECKSUM=linux
# Avoids the full build
BUILDG_CHECKSUM=buildg-v
ROOTLESSKIT_CHECKSUM=linux
SLIRP4NETNS_CHECKSUM=linux
STARGZ_SNAPSHOTTER_CHECKSUM=linux
# We specifically want the static ones
TINI_CHECKSUM=static


######################
# Lib
######################

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
  _ll="$(printf "LOG_LEVEL_%s" "${NERDCTL_CI_LOG_LEVEL:-warning}" | tr '[:lower:]' '[:upper:]')"
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
  command -v "$binary" >/dev/null || {
    log::error "You need $binary for this script to work, and it cannot be found in your path"
    exit 1
  }
}

fs::mktemp(){
  mktemp -dq "${TMPDIR:-/tmp}/$prefix.XXXXXX" 2>/dev/null || mktemp -dq || {
    log::error "Failed to create temporary directory"
    exit 1
  }
}

http::get(){
  local args=(curl --proto '=https' --tlsv1.2 -fsSL)
  args+=("$@")

  log::debug "${args[*]}"
  "${args[@]}"
}

http::checksum(){
  local urls=("$@")
  local url

  local prefix="nerdctl-checksum"

  local temp
  temp="$(fs::mktemp)"

  for url in "${urls[@]}"; do
    http::get -o "$temp/${url##*/}" "$url"
  done

  cd "$temp"
  shasum -a 256 ./*
  cd - >/dev/null || true
}

# Github API helpers
# Set GITHUB_TOKEN to use authenticated requests to workaround limitations
github::request(){
  local endpoint="$1"
  local args=(
    -H "Accept: application/vnd.github+json"
    -H "X-GitHub-Api-Version: 2022-11-28"
  )

  [ "${GITHUB_TOKEN:-}" == "" ] || args+=(-H "Authorization: Bearer $GITHUB_TOKEN")

  http::get "${args[@]}" https://api.github.com/"$endpoint"
}

github::tags::getlatest(){
  local repo="$1"
  github::request "repos/$repo/tags" |
    jq -rc .[0].name
}

github::releases::latest(){
  local repo="$1"
  github::request "repos/$repo/releases" |
    jq -rc .[]
}

version::compare(){
  local raw_version_fd="$1"
  local parsed
  local line
  while read -r line; do
    parsed+=("$line")
  done < <(sed -E 's/^(.* )?v?([0-9]+)[.]([0-9]+)([.]([0-9]+))?(-?([a-z]+)[.]?([0-9]+))?.*/\2\n\3\n\5\n\7\n\8\n/i' < "$raw_version_fd")

  local maj="${higher[0]}"
  local min="${higher[1]}"
  local patch="${higher[2]}"
  local sub="${higher[3]}"
  local subv="${higher[4]}"

  log::debug "parsed version: ${parsed[*]}"
  log::debug " > current higher version: ${higher[*]}"

  if [ "${parsed[0]}" -gt "$maj" ]; then
    log::debug " > new higher"
    higher=("${parsed[@]}")
    return
  elif [ "${parsed[0]}" -lt "$maj" ]; then
    return 1
  fi
  if [ "${parsed[1]}" -gt "$min" ]; then
    log::debug " > new higher"
    higher=("${parsed[@]}")
    return
  elif [ "${parsed[1]}" -lt "$min" ]; then
    return 1
  fi
  if [ "${parsed[2]}" -gt "$patch" ]; then
    log::debug " > new higher"
    higher=("${parsed[@]}")
    return
  elif [ "${parsed[2]}" -lt "$patch" ]; then
    return 1
  fi
  # If the current latest does not have a sub, then it is more recent
  if [ "$sub" == "" ]; then
      return 1
  fi
  # If it has a sub, and the parsed one does not, then the parsed one is more recent
  if [ "${parsed[3]}" == "" ]; then
    log::debug " > new higher"
    higher=("${parsed[@]}")
    return
  fi
  # Otherwise, we have two subs. Normalize, then compare
  # alpha < beta < rc
  [ "$sub" == "rc" ] && sub=2 || { [ "$sub" == "beta" ] && sub=1; } || { [ "$sub" == "alpha" ] && sub=0; } || {
    log::error "Unrecognized sub pattern: $sub"
    exit 42
  }
  [ "${parsed[3]}" == "rc" ] && parsed[3]=2 || { [ "${parsed[3]}" == "beta" ] && parsed[3]=1; } || { [ "${parsed[3]}" == "alpha" ] && parsed[3]=0; } || {
    log::error "Unrecognized sub pattern: ${parsed[3]}"
    exit 42
  }
  if [ "${parsed[3]}" -gt "$sub" ]; then
    log::debug " > new higher"
    higher=("${parsed[@]}")
    return
  elif [ "${parsed[3]}" -lt "$sub" ]; then
    return 1
  fi
  # Ok... we are left with just the sub version
  if [ "${parsed[4]}" -gt "$subv" ]; then
    log::debug " > new higher"
    higher=("${parsed[@]}")
    return
  elif [ "${parsed[4]}" -lt "$subv" ]; then
    return 1
  fi
}

# Retrieves the "highest version" release for a given repo
# Optional argument 2 allows to filter out unwanted release which name matches the argument
# This is useful for repo that do independent releases for assets (like buildkit dockerfiles)
latest::release(){
  local repo="$1"
  local ignore="${2:-}"
  local line
  local name

  higher=(0 0 0 "alpha" 0)
  higher_data=
  higher_readable=

  log::info "Analyzing releases for $repo"

  while read -r line; do
    [ ! "$ignore" ] || ! grep -q "$ignore" <<<"$line" || continue
    name="$(echo "$line"  | jq -rc .name)"
    if [ "$name" == "" ] || [ "$name" == null ] ; then
      log::debug " > bogus release name ($name) ignored"
      continue
    fi
    log::debug " > found release: $name"
    if version::compare <(echo "$line" | jq -rc .name); then
      higher_data="$line"
      higher_readable="$(echo "$line" | jq -rc .name | sed -E 's/(.*[ ])?(v?[0-9][0-9.a-z-]+).*/\2/')"
    fi
  done < <(github::releases::latest "$repo")

  log::info " >>> latest release detected: $higher_readable"
}

# Retrieve the latest git tag for a given repo
latest::tag(){
  local repo="$1"

  log::info "Analyzing tags for $repo"
  github::tags::getlatest "$repo"
}

# Once a latest release has been retrieved for a given project, you can get the url to the asset matching OS and ARCH
assets::get(){
  local os="$1"
  local arch="$2"
  local name=
  local found=

  while read -r line; do
    name="$(echo "$line" | jq -rc .name)"
    log::debug " >>> candidate $name"
    ! grep -qi "$os" <<<"$name" || ! grep -qi "$arch" <<<"$name" || (
      ! grep -Eqi "[.]t?g?x?z$" <<<"$name" && grep -Eqi "[.][a-z]+$" <<<"$name"
    ) || {
      found="$line"
      break
    }
  done < <(echo "$higher_data" | jq -rc .assets.[])
  [ "$found" == "" ] && {
    log::warning " >>> no asset found for $os/$arch"
  } || {
    log::info " >>> found asset for $os/$arch: $(echo "$found" | jq -rc .browser_download_url)"
    printf "%s\n" "$(echo "$found" | jq -rc .browser_download_url)"
  }
}

log::init
host::require jq
host::require curl
host::require shasum
host::require docker
host::require tput

######################
# Script
######################

docker_args=(docker build -t test-integration --target test-integration)

for dep in "${dependencies[@]}"; do
  shortname="${dep##*/}"
  [ "$shortname" != "plugins" ] || shortname="cni-plugins"
  [ "$shortname" != "fuse-overlayfs-snapshotter" ] || shortname="containerd-fuse-overlayfs"
  for bl in "${blacklist[@]}"; do
    if [ "$bl" == "$shortname" ]; then
      log::warning "Dependency $shortname is blacklisted and will be left to its currently pinned version"
      break
    fi
  done
  [ "$bl" != "$shortname" ] || continue

  shortsafename="$(printf "%s" "$shortname" | tr '[:lower:]' '[:upper:]' | tr '-' '_')"

  exclusion="${shortsafename}_EXCLUDE"
  latest::release "$dep" "${!exclusion:-}"

  # XXX containerd does not display "v" in its released versions
  [ "${higher_readable:0:1}" == v ] || higher_readable="v$higher_readable"

  checksum="${shortsafename}_CHECKSUM"
  if [ "${!checksum:-}" != "" ]; then
    # Checksum file
    checksum_file=./Dockerfile.d/SHA256SUMS.d/"${shortname}-${higher_readable}"
    if [ ! -e "$checksum_file" ]; then
      # Get assets - try first os/arch - fallback on gnu style arch otherwise
      assets=()

      # Most well behaved go projects will tag with a go os and arch
      candidate="$(assets::get "${!checksum:-}" "amd64")"
      # Then non go projects tend to use gnu style
      [ "$candidate" != "" ] || candidate="$(assets::get "" "x86_64")"
      # And then some projects which are linux only do not specify the OS
      [ "$candidate" != "" ] || candidate="$(assets::get "" "amd64")"
      [ "$candidate" == "" ] || assets+=("$candidate")

      candidate="$(assets::get "${!checksum:-}" "arm64")"
      [ "$candidate" != "" ] || candidate="$(assets::get "" "aarch64")"
      [ "$candidate" != "" ] || candidate="$(assets::get "" "arm64")"
      [ "$candidate" == "" ] || assets+=("$candidate")
      # Fallback to source if there is nothing else

      [ "${#assets[@]}" != 0 ] || candidate="$(assets::get "" "source")"
      [ "$candidate" == "" ] || assets+=("$candidate")

      # XXX very special...
      if [ "$shortsafename" == "STARGZ_SNAPSHOTTER" ]; then
        assets+=("https://raw.githubusercontent.com/containerd/stargz-snapshotter/${higher_readable}/script/config/etc/systemd/system/stargz-snapshotter.service")
      fi

      # Write the checksum for what we found
      if [ "${#assets[@]}" == 0 ]; then
        log::error "No asset found for this checksum-able dependency. Dropping off."
        exit 1
      fi
      http::checksum "${assets[@]}" > "$checksum_file"
    fi
  fi

  while read -r line; do
    # Extract value after "=" from a possible dockerfile `ARG XXX_VERSION`
    old_version=$(echo "$line" | grep "ARG ${shortsafename}_VERSION=") || true
    old_version="${old_version##*=}"
    [ "$old_version" != "" ] || continue
    # If the Dockerfile version does NOT start with a v, adapt to that
    [ "${old_version:0:1}" == "v" ] || higher_readable="${higher_readable:1}"

    if [ "$old_version" != "$higher_readable" ]; then
      log::warning "Dependency ${shortsafename} is going to use an updated version $higher_readable (currently: $old_version)"
    fi
  done < ./Dockerfile

  docker_args+=(--build-arg "${shortsafename}_VERSION=$higher_readable")
done


GO_VERSION="$(curl -fsSL "https://go.dev/dl/?mode=json&include=all" | jq -rc .[0].version)"
GO_VERSION="${GO_VERSION##*go}"
# If a release candidate, docker hub may not have the corresponding image yet.
# So, soften the version to just "rc", as they provide that as an alias to the latest available rc on their side
# See https://github.com/containerd/nerdctl/issues/3223
! grep -Eq "rc[0-9]+$" <<<"$GO_VERSION" || GO_VERSION="${GO_VERSION%rc[0-9]*}-rc"
docker_args+=(--build-arg "GO_VERSION=$GO_VERSION")

log::debug "${docker_args[*]} ."
"${docker_args[@]}" "."
