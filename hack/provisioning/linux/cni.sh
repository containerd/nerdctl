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

provision::cni::uninstall(){
  [ "$(id -u)" == 0 ] || {
    log::error "You need to be root"
    return 1
  }

  rm -Rf /opt/cni/bin
}

# provision::containerd::cni will retrieve a specific version of cni plugins and extract it in place on the host
provision::cni(){
  local version="$1"
  local arch="$2"
  local bin_sha="$3"

  cd "$(fs::mktemp "cni-install")"

  http::get::secure \
    cni.tgz \
    https://github.com/containernetworking/plugins/releases/download/"$version"/cni-plugins-linux-"$arch"-"$version".tgz \
    "$bin_sha"

  sudo mkdir -p /opt/cni/bin
  sudo tar -C /opt/cni/bin -xzf cni.tgz

  cd - >/dev/null
}

provision::cni "$1" "$2" "$3"