#!/bin/bash

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

# test-integration-rootless.sh runs COMMAND in rootless mode, inside the systemd user
# session of the current, unprivileged user.
#
# It must be started as an unprivileged user with passwordless sudo, on a host
# provisioned with hack/provisioning/linux/test-integration-env.sh (which, among other
# things, enables lingering for the user, so that the systemd user session is available
# even when this script does not run from a logind session, eg: on a GitHub Actions
# runner).
#
# Usage: test-integration-rootless.sh COMMAND [ARGS...]
set -eux -o pipefail

[ "$(id -u)" != "0" ] || {
	echo "This script must be started as an unprivileged user with passwordless sudo" >&2
	exit 1
}

# This script substantially and irreversibly modifies the host (and the current user
# account): it is only safe to run on a disposable CI machine
[ "${GITHUB_ACTIONS:-}" == "true" ] || {
	echo "Refusing to run outside of GitHub Actions (export GITHUB_ACTIONS=true to force)" >&2
	exit 1
}

# systemctl --user (and the dbus clients) locate the systemd user session through
# XDG_RUNTIME_DIR and DBUS_SESSION_BUS_ADDRESS. They are inherited when the script runs
# from a logind session (eg: an ssh session into a Lima guest), but not necessarily
# otherwise (eg: a GitHub Actions runner job): if unset, default them to the standard
# locations of the user session, which exists in any case, as the provisioning script
# enabled lingering.
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
export DBUS_SESSION_BUS_ADDRESS="${DBUS_SESSION_BUS_ADDRESS:-unix:path=${XDG_RUNTIME_DIR}/bus}"

# The environment below mirrors Lima's 20-rootless-base.sh boot script, which does not
# run in the CI guest VMs (they are started in plain mode).
# Make sure iptables and mount.fuse3 are resolvable (on EL, non-login shells do not get
# the sbin directories in their PATH).
export PATH="$PATH:/usr/sbin:/sbin"
# fuse-overlayfs is the most stable snapshotter for rootless, on kernel < 5.13 (eg: EL 8)
# https://rootlesscontaine.rs/how-it-works/overlayfs/
if [ -z "${CONTAINERD_SNAPSHOTTER:-}" ]; then
	kernel="$(uname -r)"
	kernel="${kernel%%-*}"
	if [ "$(printf '%s\n' "$kernel" "5.13" | sort -V | head -n1)" != "5.13" ]; then
		export CONTAINERD_SNAPSHOTTER="fuse-overlayfs"
	fi
fi

export IPFS_PATH="$HOME/.local/share/ipfs"

# If anything fails below, the systemd user journal usually knows why
trap 'sudo journalctl --no-pager --lines=200 _UID="$(id -u)" >&2 || true' ERR

# This script gets invoked repeatedly (eg: non-flaky, then flaky test runs).
# The installation below is not idempotent (specifically, the containerd configuration
# must not be appended twice), so, only perform it once.
if [ ! -e "$HOME/.config/nerdctl-test-setup-done" ]; then
	# The rootlesskit port driver is configured through the environment of the (generated)
	# containerd systemd user unit, so, it has to be baked into a unit drop-in.
	if [ "${CONTAINERD_ROOTLESS_ROOTLESSKIT_PORT_DRIVER:-builtin}" != "builtin" ]; then
		mkdir -p "$HOME/.config/systemd/user/containerd.service.d"
		cat <<-EOF >"$HOME/.config/systemd/user/containerd.service.d/port-driver.conf"
		[Service]
		Environment="CONTAINERD_ROOTLESS_ROOTLESSKIT_PORT_DRIVER=${CONTAINERD_ROOTLESS_ROOTLESSKIT_PORT_DRIVER}"
		EOF
		systemctl --user daemon-reload
	fi

	if [ "${CONTAINERD_ROOTLESS_ROOTLESSKIT_IPV6:-false}" = "true" ]; then
		mkdir -p "$HOME/.config/systemd/user/containerd.service.d"
		cat <<-EOF >"$HOME/.config/systemd/user/containerd.service.d/ipv6.conf"
		[Service]
		Environment="CONTAINERD_ROOTLESS_ROOTLESSKIT_IPV6=true"
		EOF
		systemctl --user daemon-reload
	fi

	containerd-rootless-setuptool.sh install
	if grep -q "options use-vc" /etc/resolv.conf; then
		containerd-rootless-setuptool.sh nsenter -- sh -euc 'echo "options use-vc" >>/etc/resolv.conf'
	fi

	if [ "${WORKAROUND_ISSUE_622:-}" != "" ]; then
		echo "WORKAROUND_ISSUE_622: Not enabling BuildKit (https://github.com/containerd/nerdctl/issues/622)" >&2
	else
		CONTAINERD_NAMESPACE="nerdctl-test" containerd-rootless-setuptool.sh install-buildkit-containerd
	fi
	containerd-rootless-setuptool.sh install-stargz
	# The fuse-overlayfs snapshotter is required on hosts that cannot mount overlayfs
	# in a user namespace (eg: EL 8)
	containerd-rootless-setuptool.sh install-fuse-overlayfs
	if [ ! -f "$HOME/.config/containerd/config.toml" ]; then
		mkdir -p "$HOME/.config/containerd"
		echo "version = 2" >"$HOME/.config/containerd/config.toml"
	fi
	cat <<EOF >>"$HOME/.config/containerd/config.toml"
[proxy_plugins]
  [proxy_plugins."stargz"]
    type = "snapshot"
    address = "/run/user/$(id -u)/containerd-stargz-grpc/containerd-stargz-grpc.sock"
  [proxy_plugins.stargz.exports]
    root = "$HOME/.local/share/containerd-stargz-grpc/"
    enable_remote_snapshot_annotations = "true"
  [proxy_plugins."fuse-overlayfs"]
    type = "snapshot"
    address = "/run/user/$(id -u)/containerd-fuse-overlayfs.sock"
  [proxy_plugins."fuse-overlayfs".exports]
    root = "$HOME/.local/share/containerd-fuse-overlayfs/"
[[plugins."io.containerd.transfer.v1.local".unpack_config]]
  platform = "linux"
  snapshotter = "overlayfs"
[[plugins."io.containerd.transfer.v1.local".unpack_config]]
  platform = "linux"
  snapshotter = "stargz"
[[plugins."io.containerd.transfer.v1.local".unpack_config]]
  platform = "linux"
  snapshotter = "fuse-overlayfs"
EOF
	systemctl --user restart containerd.service
	containerd-rootless-setuptool.sh -- install-ipfs --init --offline # offline ipfs daemon for testing
	echo "ipfs = true" >>"$HOME/.config/containerd-stargz-grpc/config.toml"
	systemctl --user restart stargz-snapshotter.service
	containerd-rootless-setuptool.sh install-bypass4netnsd

	touch "$HOME/.config/nerdctl-test-setup-done"
fi

exec "$@"
