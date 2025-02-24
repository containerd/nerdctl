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

set -eux -o pipefail
if [[ "$(id -u)" = "0" ]]; then
  # Ensure securityfs is mounted for apparmor to work
  if ! mountpoint -q /sys/kernel/security; then
    mount -tsecurityfs securityfs /sys/kernel/security
  fi
	if [ -e /sys/kernel/security/apparmor/profiles ]; then
		# Load the "nerdctl-default" profile for TestRunApparmor
		nerdctl apparmor load
	fi

	: "${WORKAROUND_ISSUE_622:=}"
	if [[ "$WORKAROUND_ISSUE_622" = "1" ]]; then
		touch /workaround-issue-622
	fi

	# Switch to the rootless user via SSH
	systemctl start ssh
	exec ssh -o StrictHostKeyChecking=no rootless@localhost "$0" "$@"
else
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
	if [ ! -f "/home/rootless/.config/containerd/config.toml" ] ; then
		echo "version = 2" > /home/rootless/.config/containerd/config.toml
	fi
	cat <<EOF >>/home/rootless/.config/containerd/config.toml
[proxy_plugins]
  [proxy_plugins."stargz"]
    type = "snapshot"
    address = "/run/user/$(id -u)/containerd-stargz-grpc/containerd-stargz-grpc.sock"
EOF
	systemctl --user restart containerd.service
	containerd-rootless-setuptool.sh -- install-ipfs --init --offline # offline ipfs daemon for testing
	echo "ipfs = true" >>/home/rootless/.config/containerd-stargz-grpc/config.toml
	systemctl --user restart stargz-snapshotter.service
	export IPFS_PATH="/home/rootless/.local/share/ipfs"
	containerd-rootless-setuptool.sh install-bypass4netnsd
	# Once ssh-ed, we lost the Dockerfile working dir, so, get back in the nerdctl checkout
	cd /go/src/github.com/containerd/nerdctl
	# We also lose the PATH (and SendEnv=PATH would require sshd config changes)
	exec env PATH="/usr/local/go/bin:$PATH" "$@"
fi
