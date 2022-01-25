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
	if [ -e /sys/kernel/security/apparmor/profiles ]; then
		# Load the "nerdctl-default" profile for TestRunApparmor
		nerdctl apparmor load
	fi

	# Switch to the rootless user via SSH
	systemctl start sshd
	exec ssh -o StrictHostKeyChecking=no rootless@localhost "$0" "$@"
else
	containerd-rootless-setuptool.sh install
	if grep -q "options use-vc" /etc/resolv.conf; then
		containerd-rootless-setuptool.sh nsenter -- sh -euc 'echo "options use-vc" >>/etc/resolv.conf'
	fi
	containerd-rootless-setuptool.sh install-buildkit
	containerd-rootless-setuptool.sh install-stargz
	cat <<EOF >>/home/rootless/.config/containerd/config.toml
[proxy_plugins]
  [proxy_plugins."stargz"]
    type = "snapshot"
    address = "/run/user/1000/containerd-stargz-grpc/containerd-stargz-grpc.sock"
EOF
	systemctl --user restart containerd.service
	containerd-rootless-setuptool.sh -- install-ipfs --init --offline # offline ipfs daemon for testing
	echo "ipfs = true" >>/home/rootless/.config/containerd-stargz-grpc/config.toml
	systemctl --user restart stargz-snapshotter.service
	export IPFS_PATH="/home/rootless/.local/share/ipfs"
	exec "$@"
fi
