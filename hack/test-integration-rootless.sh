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

# test-integration-rootless.sh runs COMMAND in rootless mode, inside a fresh systemd
# user session obtained by ssh-ing back into the current, unprivileged user (`sudo`
# cannot create the session, and `machinectl shell` does not propagate the exit code).
#
# It must be started as an unprivileged user with passwordless sudo, on a host
# provisioned with hack/provisioning/linux/test-integration-env.sh.
#
# Usage: test-integration-rootless.sh COMMAND [ARGS...]
set -eux -o pipefail

# The first phase communicates additional environment (typically PATH, which is reset by the
# ssh login) to the second phase through this file.
readonly env_file=".config/nerdctl-test-env"

if [ "${1:-}" = "internal-exec" ]; then
	# Second phase: we are now inside a systemd user session (created by ssh-ing into an
	# unprivileged user).
	shift
	workdir="$1"
	shift

	# shellcheck source=/dev/null
	[ ! -e "$HOME/$env_file" ] || . "$HOME/$env_file"

	# This script gets invoked repeatedly (eg: non-flaky, then flaky test runs).
	# The installation below is not idempotent (specifically, the containerd configuration
	# must not be appended twice), so, only perform it once.
	if [ -e "$HOME/.config/nerdctl-test-setup-done" ]; then
		export IPFS_PATH="$HOME/.local/share/ipfs"
		cd "$workdir"
		exec env PATH="/usr/local/go/bin:$PATH" "$@"
	fi

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

	containerd-rootless-setuptool.sh install
	if grep -q "options use-vc" /etc/resolv.conf; then
		containerd-rootless-setuptool.sh nsenter -- sh -euc 'echo "options use-vc" >>/etc/resolv.conf'
	fi

	if [[ -e /workaround-issue-622 ]]; then
		echo "WORKAROUND_ISSUE_622: Not enabling BuildKit (https://github.com/containerd/nerdctl/issues/622)" >&2
	else
		CONTAINERD_NAMESPACE="nerdctl-test" containerd-rootless-setuptool.sh install-buildkit-containerd
	fi
	containerd-rootless-setuptool.sh install-stargz
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
[[plugins."io.containerd.transfer.v1.local".unpack_config]]
  platform = "linux"
  snapshotter = "overlayfs"
[[plugins."io.containerd.transfer.v1.local".unpack_config]]
  platform = "linux"
  snapshotter = "stargz"
EOF
	systemctl --user restart containerd.service
	containerd-rootless-setuptool.sh -- install-ipfs --init --offline # offline ipfs daemon for testing
	echo "ipfs = true" >>"$HOME/.config/containerd-stargz-grpc/config.toml"
	systemctl --user restart stargz-snapshotter.service
	export IPFS_PATH="$HOME/.local/share/ipfs"
	containerd-rootless-setuptool.sh install-bypass4netnsd

	touch "$HOME/.config/nerdctl-test-setup-done"

	cd "$workdir"
	exec env PATH="/usr/local/go/bin:$PATH" "$@"
fi

# First phase: prepare the environment through sudo, then ssh back into the current,
# unprivileged user to obtain a systemd user session, and hand over to the second phase.
[ "$(id -u)" != "0" ] || {
	echo "This script must be started as an unprivileged user with passwordless sudo" >&2
	exit 1
}

# Ensure securityfs is mounted for apparmor to work
if ! sudo mountpoint -q /sys/kernel/security; then
	sudo mount -tsecurityfs securityfs /sys/kernel/security
fi
if [ -e /sys/kernel/security/apparmor/profiles ]; then
	# Load the "nerdctl-default" profile for TestRunApparmor
	sudo nerdctl apparmor load
fi

: "${WORKAROUND_ISSUE_622:=}"
if [[ "$WORKAROUND_ISSUE_622" != "" ]]; then
	sudo touch /workaround-issue-622
fi

# Pass the current environment over to the ssh session
mkdir -p "$(dirname "$HOME/$env_file")"
printf 'export PATH="%s"\n' "$PATH" >"$HOME/$env_file"
if [ "${CONTAINERD_ROOTLESS_ROOTLESSKIT_PORT_DRIVER:-}" != "" ]; then
	printf 'export CONTAINERD_ROOTLESS_ROOTLESSKIT_PORT_DRIVER="%s"\n' "$CONTAINERD_ROOTLESS_ROOTLESSKIT_PORT_DRIVER" >>"$HOME/$env_file"
fi
if [ "${GITHUB_STEP_SUMMARY:-}" != "" ]; then
	# The summary file lives on the same machine, so it can be written to from the ssh session
	printf 'export GITHUB_STEP_SUMMARY="%s"\n' "$GITHUB_STEP_SUMMARY" >>"$HOME/$env_file"
fi

# Make sure we can actually ssh into ourselves
mkdir -p "$HOME/.ssh" && chmod 0700 "$HOME/.ssh"
if [ ! -e "$HOME/.ssh/id_ed25519" ]; then
	ssh-keygen -q -t ed25519 -f "$HOME/.ssh/id_ed25519" -N ''
fi
touch "$HOME/.ssh/authorized_keys"
grep -qF "$(cat "$HOME/.ssh/id_ed25519.pub")" "$HOME/.ssh/authorized_keys" \
	|| cat "$HOME/.ssh/id_ed25519.pub" >>"$HOME/.ssh/authorized_keys"

# The ssh unit is named "ssh" on Ubuntu/Debian, and "sshd" on EL
sudo systemctl start ssh 2>/dev/null || sudo systemctl start sshd

# The ssh session starts from $HOME: resolve this script to an absolute path
exec ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "$(id -un)@localhost" "$(realpath "$0")" internal-exec "$(pwd)" "$@"
