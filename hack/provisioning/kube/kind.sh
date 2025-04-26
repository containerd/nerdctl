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

GO_VERSION=1.24
KIND_VERSION=v0.27.0
CNI_PLUGINS_VERSION=v1.7.1
# shellcheck disable=SC2034
CNI_PLUGINS_SHA_AMD64=1a28a0506bfe5bcdc981caf1a49eeab7e72da8321f1119b7be85f22621013098
# shellcheck disable=SC2034
CNI_PLUGINS_SHA_ARM64=119fcb508d1ac2149e49a550752f9cd64d023a1d70e189b59c476e4d2bf7c497

[ "$(uname -m)" == "aarch64" ] && GOARCH=arm64 || GOARCH=amd64

_rootful=

configure::rootful(){
  log::debug "Configuring rootful to: ${1:+true}"
  _rootful="${1:+true}"
}

install::kind(){
  local version="$1"
  local temp
  temp="$(fs::mktemp "install")"

  http::get "$temp"/kind "https://kind.sigs.k8s.io/dl/$version/kind-linux-${GOARCH:-amd64}"
  host::install "$temp"/kind
}

# shellcheck disable=SC2120
install::kubectl(){
  local version="${1:-}"
  [ "$version" ] || version="$(http::get /dev/stdout https://dl.k8s.io/release/stable.txt)"
  local temp
  temp="$(fs::mktemp "install")"

  http::get "$temp"/kubectl "https://dl.k8s.io/release/$version/bin/linux/${GOARCH:-amd64}/kubectl"

  host::install "$temp"/kubectl
}

exec::kind(){
  local args=()
  [ ! "$_rootful" ] || args=(sudo env PATH="$PATH" KIND_EXPERIMENTAL_PROVIDER="$KIND_EXPERIMENTAL_PROVIDER")
  args+=(kind)

  log::debug "${args[*]} $*"
  "${args[@]}" "$@"
}

exec::nerdctl(){
  local args=()
  [ ! "$_rootful" ] || args=(sudo env PATH="$PATH")
  args+=("$(pwd)"/_output/nerdctl)

  log::debug "${args[*]} $*"
  "${args[@]}" "$@"
}

# Install dependencies
main(){
  log::info "Configuring rootful"
  configure::rootful "${ROOTFUL:-}"

  log::info "Installing host dependencies if necessary"
  host::require make go
  host::require kind 2>/dev/null || install::kind "$KIND_VERSION"
  host::require kubectl 2>/dev/null || install::kubectl

  # Build nerdctl to use for kind
  make -f "$root/../../../Makefile" binaries
  PATH=$(pwd)/_output:"$PATH"
  export PATH

  # Add CNI plugins
  local sha
  sha="CNI_PLUGINS_SHA_$(tr "[:lower:]" "[:upper:]" <<<"$GOARCH")"
  # shellcheck source=/dev/null
  "$root"/../linux/cni.sh "$CNI_PLUGINS_VERSION" "$GOARCH" "${!sha}"

  # Hack to get go into kind control plane
  exec::nerdctl rm -f go-kind 2>/dev/null || true
  exec::nerdctl run -d --quiet --name go-kind golang:"$GO_VERSION" sleep Inf
  exec::nerdctl cp go-kind:/usr/local/go /tmp/go
  exec::nerdctl rm -f go-kind

  # Create fresh cluster
  log::info "Creating new cluster"
  export KIND_EXPERIMENTAL_PROVIDER=nerdctl
  exec::kind delete cluster --name nerdctl-test 2>/dev/null || true
  exec::kind create cluster --name nerdctl-test --config="$root"/kind.yaml
}

main "$@"
