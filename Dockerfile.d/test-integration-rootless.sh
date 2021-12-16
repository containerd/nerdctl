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

	: "${DISABLE_SLIRP4NETNS_DNS:=}"
	if [[ "$DISABLE_SLIRP4NETNS_DNS" = "1" ]]; then
		cat <<EOF >/tmp/resolv.conf.no_slirp4netns
# Workaround for https://github.com/containerd/nerdctl/issues/622
# ERROR: failed to do request: Head "https://ghcr.io/v2/stargz-containers/alpine/manifests/3.13-org": dial tcp: lookup ghcr.io on 10.0.2.3:53: read udp 10.0.2.100:50602->10.0.2.3:53: i/o timeout
nameserver 8.8.8.8
EOF
	fi

	# Switch to the rootless user via SSH
	systemctl start sshd
	exec ssh -o StrictHostKeyChecking=no rootless@localhost "$0" "$@"
else
	containerd-rootless-setuptool.sh install
	if [[ -f /tmp/resolv.conf.no_slirp4netns ]]; then
		containerd-rootless-setuptool.sh nsenter -- cp -f /tmp/resolv.conf.no_slirp4netns /etc/resolv.conf
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
