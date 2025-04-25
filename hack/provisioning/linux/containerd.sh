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
root="$(cd "$(dirname "${BASH_SOURCE[0]:-$PWD}")" 2>/dev/null 1>&2 && pwd)"
readonly root
# shellcheck source=/dev/null
. "$root/../../scripts/lib.sh"

# provision::containerd::uninstall will ensure deb containerd are purged
provision::containerd::uninstall(){
  [ "$(id -u)" == 0 ] || {
    log::error "You need to be root"
    return 1
  }

  # Purge deb package
  apt-get -q purge containerd* 2>/dev/null || true
  # Remove conf
  rm -f /etc/containerd/containerd.toml
  # Remove manually installed containerd if leftover
  systemctl stop containerd 2>/dev/null
  rm -f /lib/systemd/system/containerd.service
  systemctl daemon-reload 2>/dev/null || true
  ! command -v containerd || rm -f "$(which containerd)"
}

# provision::containerd::rootful will retrieve a specific version of containerd and install it on the host.
provision::containerd::rootful(){
  local version="$1"
  local arch="$2"
  local bin_sha="$3"
  local service_sha="$4"

  # Be tolerant with passed versions - with or without leading "v"
  [ "${version:0:1}" != "v" ] || version="${version:1}"

  cd "$(fs::mktemp "containerd-install")"

  # Get the binary and install it
  http::get::secure \
    containerd.tar.gz \
    https://github.com/containerd/containerd/releases/download/v"$version"/containerd-"$version"-linux-"$arch".tar.gz \
    "$bin_sha"

  sudo tar::expand /usr/local containerd.tar.gz

  # Get the systemd unit
  http::get::secure \
    containerd.service \
    https://raw.githubusercontent.com/containerd/containerd/refs/tags/v"$version"/containerd.service \
    "$service_sha"

  sudo cp containerd.service /lib/systemd/system/containerd.service

  # Start it
  sudo systemctl daemon-reload
  sudo systemctl start containerd

  cd - >/dev/null || true
}

provision::containerd::rootful "$1" "$2" "$3" "$4"