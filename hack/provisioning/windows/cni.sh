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

# adapted from: https://raw.githubusercontent.com/containerd/containerd/refs/tags/v2.0.3/script/setup/install-cni-windows

set -o errexit -o errtrace -o functrace -o nounset -o pipefail

WINCNI_VERSION="${WINCNI_VERSION:-v0.3.1}"

git config --global advice.detachedHead false

DESTDIR="${DESTDIR:-"C:\\Program Files\\containerd\\cni"}"
WINCNI_BIN_DIR="${DESTDIR}/bin"
WINCNI_PKG=github.com/Microsoft/windows-container-networking

git clone --quiet --depth 1 --branch "${WINCNI_VERSION}" "https://${WINCNI_PKG}.git" "${GOPATH}/src/${WINCNI_PKG}"
cd "${GOPATH}/src/${WINCNI_PKG}"
make all
install -D -m 755 "out/nat.exe" "${WINCNI_BIN_DIR}/nat.exe"
install -D -m 755 "out/sdnbridge.exe" "${WINCNI_BIN_DIR}/sdnbridge.exe"
install -D -m 755 "out/sdnoverlay.exe" "${WINCNI_BIN_DIR}/sdnoverlay.exe"

CNI_CONFIG_DIR="${DESTDIR}/conf"
mkdir -p "${CNI_CONFIG_DIR}"

# split_ip splits ip into a 4-element array.
split_ip() {
  local -r varname="$1"
  local -r ip="$2"
  for i in {0..3}; do
    eval "$varname"["$i"]="$( echo "$ip" | cut -d '.' -f $((i + 1)) )"
  done
}

# subnet gets subnet for a gateway, e.g. 192.168.100.0/24.
calculate_subnet() {
  local -r gateway="$1"
  local -r prefix_len="$2"
  split_ip gateway_array "$gateway"
  local len=$prefix_len
  for i in {0..3}; do
    if (( len >= 8 )); then
      mask=255
    elif (( len > 0 )); then
      mask=$(( 256 - 2 ** ( 8 - len ) ))
    else
      mask=0
    fi
    (( len -= 8 ))
    #shellcheck disable=SC2154
    result_array[i]=$(( gateway_array[i] & mask ))
  done
  result="$(printf ".%s" "${result_array[@]}")"
  result="${result:1}"
  echo "$result/$((32 - prefix_len))"
}

# nat already exists on the Windows VM, the subnet and gateway
# we specify should match that.
: "${GATEWAY:=$(powershell -c "(Get-NetIPAddress -InterfaceAlias 'vEthernet (nat)' -AddressFamily IPv4).IPAddress")}"
: "${PREFIX_LEN:=$(powershell -c "(Get-NetIPAddress -InterfaceAlias 'vEthernet (nat)' -AddressFamily IPv4).PrefixLength")}"

subnet="$(calculate_subnet "$GATEWAY" "$PREFIX_LEN")"

# The "name" field in the config is used as the underlying
# network type right now (see
# https://github.com/microsoft/windows-container-networking/pull/45),
# so it must match a network type in:
# https://docs.microsoft.com/en-us/windows-server/networking/technologies/hcn/hcn-json-document-schemas
bash -c 'cat >"'"${CNI_CONFIG_DIR}"'"/0-containerd-nat.conflist <<EOF
{
  "cniVersion": "1.0.0",
  "name": "nat",
  "plugins": [
    {
      "type": "nat",
      "master": "Ethernet",
      "ipam": {
        "subnet": "'"$subnet"'",
        "routes": [
          {
            "GW": "'"$GATEWAY"'"
          }
        ]
      },
      "capabilities": {
        "portMappings": true,
        "dns": true
      }
    }
  ]
}
EOF'