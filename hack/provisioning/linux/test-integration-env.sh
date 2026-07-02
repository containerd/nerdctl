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

# test-integration-env.sh provisions a linux host (a GitHub Actions runner, or a Lima guest)
# so that the integration test suite can run directly on it with `go test`
# (through hack/test-integration.sh), without being wrapped inside a Docker container.
#
# It expects a directory containing the artifacts exported from the
# `out-test-integration-artifacts` Dockerfile stage - Docker is only used to *build* them:
#   docker buildx build --target out-test-integration-artifacts --output type=local,dest=DIR .
#
# Usage (as root, through sudo from the unprivileged user meant to run the tests):
#   sudo ./hack/provisioning/linux/test-integration-env.sh install DIR [rootful|rootless|rootless-port-slirp4netns]
#
# Supported distributions: Ubuntu (GitHub Actions runners), and Enterprise Linux (Lima guests).

set -o errexit -o errtrace -o functrace -o nounset -o pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]:-$PWD}")" 2>/dev/null 1>&2 && pwd)"
readonly root
# shellcheck source=/dev/null
. "$root/../../scripts/lib.sh"

readonly repo_root="$root/../../.."

host::packages(){
  if command -v apt-get >/dev/null; then
    # `expect` package contains `unbuffer(1)`, which is used for emulating TTY for testing
    # `jq` is required to generate test summaries
    apt-get update -qq >/dev/null
    add-apt-repository -y ppa:criu/ppa >/dev/null
    apt-get install -qq --no-install-recommends \
      apparmor \
      criu \
      dbus-user-session \
      expect \
      fuse3 \
      git \
      jq \
      make \
      openssh-server \
      uidmap >/dev/null
  elif command -v dnf >/dev/null; then
    dnf install -q -y \
      criu \
      expect \
      fuse3 \
      git \
      iptables \
      jq \
      make \
      openssh-server \
      shadow-utils \
      tar
  else
    log::error "Unsupported distribution (neither apt-get nor dnf found)"
    return 1
  fi
}

host::slirp4netns(){
  if command -v apt-get >/dev/null; then
    apt-get install -qq --no-install-recommends slirp4netns >/dev/null
  else
    dnf install -q -y slirp4netns
  fi
}

host::artifacts(){
  local artifacts="$1"

  # The distribution-shipped containerd and Docker (if any) conflict with the containerd
  # under test. Note that Docker cannot be used anymore past this point.
  systemctl disable --now docker.socket 2>/dev/null || true
  systemctl disable --now docker.service 2>/dev/null || true
  systemctl disable --now containerd.service 2>/dev/null || true

  # /usr/local/lib/systemd/system is part of the default systemd unit search path,
  # so, the containerd, buildkit and stargz-snapshotter units are usable right away.
  # --no-overwrite-dir keeps the metadata of pre-existing directories (notably the
  # permissions of /usr/local itself - the buildx local exporter creates the artifacts
  # directory with mode 0700).
  # --no-same-owner makes the files owned by root, rather than by the user that ran buildx.
  (cd "$artifacts" && tar -cf- .) | tar -C /usr/local -xf- --no-same-owner --no-overwrite-dir
}

host::configuration(){
  # Test-specific containerd, buildkit, stargz and soci configurations
  mkdir -p /etc/containerd /etc/buildkit /etc/containerd-stargz-grpc /etc/soci-snapshotter-grpc
  cp "$repo_root/Dockerfile.d/test-integration-etc_containerd_config.toml" /etc/containerd/config.toml
  cp "$repo_root/Dockerfile.d/etc_buildkit_buildkitd.toml" /etc/buildkit/buildkitd.toml
  cp "$repo_root/Dockerfile.d/test-integration-etc_containerd-stargz-grpc_config.toml" /etc/containerd-stargz-grpc/config.toml
  printf '\n[pull_modes]\n  [pull_modes.soci_v1]\n    enable = true\n' > /etc/soci-snapshotter-grpc/config.toml
  mkdir -p /etc/cni && chmod 0755 /etc/cni

  # Test-specific systemd units (they are started explicitly - their [Install] section
  # references the docker-entrypoint.target used by the test image)
  cp "$repo_root"/Dockerfile.d/test-integration-*.service /etc/systemd/system/

  # On EL, be permissive: the test environment is not currently designed to run enforcing
  # (the containerized test environment used to run privileged)
  if command -v setenforce >/dev/null; then
    setenforce 0 || true
    [ ! -e /etc/selinux/config ] || sed -i 's/^SELINUX=enforcing/SELINUX=permissive/' /etc/selinux/config
  fi

  # Tests are run by an unprivileged user, wrapping privileged invocations with
  # `sudo`: the binaries under test must be resolvable then.
  printf 'Defaults secure_path="/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"\n' \
    > /etc/sudoers.d/99-test-integration
  chmod 0440 /etc/sudoers.d/99-test-integration
  # Note: only validate our own drop-in - other, pre-existing files might not be valid
  # (eg: /etc/sudoers.d/runner on GitHub Actions runners has "bad" permissions)
  visudo -c -f /etc/sudoers.d/99-test-integration >/dev/null

  # This ensures that bridged traffic goes through netfilter
  modprobe br-netfilter || log::warning "Failed to load the br-netfilter module"
}

host::services(){
  local target="$1"
  local unit
  local units=(
    test-integration-soci-snapshotter.service
    containerd.service
    buildkit.service
    stargz-snapshotter.service
    test-integration-buildkit-nerdctl-test.service
  )

  if [ "$target" == "rootful" ]; then
    # Offline ipfs daemon for testing. Avoid using 5001(api)/8080(gateway) which are
    # reserved by tests. In rootless mode, this is handled by containerd-rootless-setuptool.sh.
    if [ ! -e /root/.ipfs/config ]; then
      env IPFS_PATH=/root/.ipfs /usr/local/bin/ipfs init >/dev/null
    fi
    env IPFS_PATH=/root/.ipfs /usr/local/bin/ipfs config Addresses.API "/ip4/127.0.0.1/tcp/5888"
    env IPFS_PATH=/root/.ipfs /usr/local/bin/ipfs config Addresses.Gateway "/ip4/127.0.0.1/tcp/5889"
    units+=(test-integration-ipfs-offline.service)
  fi

  systemctl daemon-reload
  for unit in "${units[@]}"; do
    systemctl restart "$unit"
  done
}

host::rootless(){
  local target="$1"
  # The unprivileged user that is going to run the tests
  local user="${SUDO_USER:-}"

  if [ "$user" == "" ] || [ "$user" == "root" ]; then
    log::error "The $target target requires this script to be run with sudo, from the unprivileged user meant to run the tests"
    return 1
  fi

  # Rootless containerd needs subordinate uids/gids for the testing user
  grep -q "^$user:" /etc/subuid 2>/dev/null || echo "$user:100000:65536" >> /etc/subuid
  grep -q "^$user:" /etc/subgid 2>/dev/null || echo "$user:100000:65536" >> /etc/subgid

  # Since Ubuntu 23.10+, apparmor restricts unprivileged user namespaces creation
  if [ -e /etc/apparmor.d/abi/4.0 ]; then
    cat <<EOT > "/etc/apparmor.d/usr.local.bin.rootlesskit"
abi <abi/4.0>,
include <tunables/global>
/usr/local/bin/rootlesskit flags=(unconfined) {
  userns,
  # Site-specific additions and overrides. See local/README for details.
  include if exists <local/usr.local.bin.rootlesskit>
}
EOT
    systemctl restart apparmor.service
  fi

  # cgroup v2 delegation, for resource limits to work in rootless mode
  mkdir -p /etc/systemd/system/user@.service.d
  cp "$repo_root/Dockerfile.d/etc_systemd_system_user@.service.d_delegate.conf" /etc/systemd/system/user@.service.d/delegate.conf
  systemctl daemon-reload

  # Keep the systemd user session (and thus the rootless daemons installed by
  # containerd-rootless-setuptool.sh) alive in-between ssh sessions
  loginctl enable-linger "$user"

  # slirp4netns is required by the slirp4netns port driver, and by rootlesskit prior to v3.0
  if [ "$target" == "rootless-port-slirp4netns" ] || ! /usr/local/bin/rootlesskit --help | grep -q gvisor-tap-vsock; then
    host::slirp4netns
  fi
}

provision::test-integration-env::install(){
  local artifacts="${1:-}"
  local target="${2:-rootful}"

  [ "$(id -u)" == 0 ] || {
    log::error "You need to be root"
    return 1
  }

  if [ "$artifacts" == "" ] || [ ! -d "$artifacts" ]; then
    log::error "You need to point at a directory containing the test artifacts (built with: docker buildx build --target out-test-integration-artifacts --output type=local,dest=DIR .)"
    return 1
  fi

  host::packages
  host::artifacts "$artifacts"
  host::configuration
  [ "$target" == "rootful" ] || host::rootless "$target"
  host::services "$target"
}

com="$1"
shift
provision::test-integration-env::"$com" "$@"
