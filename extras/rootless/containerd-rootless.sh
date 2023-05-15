#!/bin/sh

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

# -----------------------------------------------------------------------------
# Forked from https://github.com/moby/moby/blob/v20.10.3/contrib/dockerd-rootless.sh
# Copyright The Moby Authors.
# Licensed under the Apache License, Version 2.0
# NOTICE: https://github.com/moby/moby/blob/v20.10.3/NOTICE
# -----------------------------------------------------------------------------

# containerd-rootless.sh executes containerd in rootless mode.
#
# Usage: containerd-rootless.sh [CONTAINERD_OPTIONS]
#
# External dependencies:
# * newuidmap and newgidmap needs to be installed.
# * /etc/subuid and /etc/subgid needs to be configured for the current user.
# * RootlessKit (>= v0.10.0) needs to be installed. RootlessKit >= v0.14.1 is recommended.
# * Either one of slirp4netns (>= v0.4.0), VPNKit, lxc-user-nic needs to be installed. slirp4netns >= v1.1.7 is recommended.
#
# Recognized environment variables:
# * CONTAINERD_ROOTLESS_ROOTLESSKIT_STATE_DIR=DIR: the rootlesskit state dir. Defaults to "$XDG_RUNTIME_DIR/containerd-rootless".
# * CONTAINERD_ROOTLESS_ROOTLESSKIT_NET=(slirp4netns|vpnkit|lxc-user-nic): the rootlesskit network driver. Defaults to "slirp4netns" if slirp4netns (>= v0.4.0) is installed. Otherwise defaults to "vpnkit".
# * CONTAINERD_ROOTLESS_ROOTLESSKIT_MTU=NUM: the MTU value for the rootlesskit network driver. Defaults to 65520 for slirp4netns, 1500 for other drivers.
# * CONTAINERD_ROOTLESS_ROOTLESSKIT_PORT_DRIVER=(builtin|slirp4netns): the rootlesskit port driver. Defaults to "builtin".
# * CONTAINERD_ROOTLESS_ROOTLESSKIT_SLIRP4NETNS_SANDBOX=(auto|true|false): whether to protect slirp4netns with a dedicated mount namespace. Defaults to "auto".
# * CONTAINERD_ROOTLESS_ROOTLESSKIT_SLIRP4NETNS_SECCOMP=(auto|true|false): whether to protect slirp4netns with seccomp. Defaults to "auto".

set -e
if ! [ -w $XDG_RUNTIME_DIR ]; then
	echo "XDG_RUNTIME_DIR needs to be set and writable"
	exit 1
fi
if ! [ -w $HOME ]; then
	echo "HOME needs to be set and writable"
	exit 1
fi
: "${XDG_DATA_HOME:=$HOME/.local/share}"
: "${XDG_CONFIG_HOME:=$HOME/.config}"

if [ -z $_CONTAINERD_ROOTLESS_CHILD ]; then
	if [ "$(id -u)" = "0" ]; then
		echo "Must not run as root"
		exit 1
	fi
	case "$1" in
	"check" | "install" | "uninstall")
		echo "Did you mean 'containerd-rootless-setuptool.sh $@' ?"
		exit 1
		;;
	esac

	: "${CONTAINERD_ROOTLESS_ROOTLESSKIT_STATE_DIR:=$XDG_RUNTIME_DIR/containerd-rootless}"
	: "${CONTAINERD_ROOTLESS_ROOTLESSKIT_NET:=}"
	: "${CONTAINERD_ROOTLESS_ROOTLESSKIT_MTU:=}"
	: "${CONTAINERD_ROOTLESS_ROOTLESSKIT_PORT_DRIVER:=builtin}"
	: "${CONTAINERD_ROOTLESS_ROOTLESSKIT_SLIRP4NETNS_SANDBOX:=auto}"
	: "${CONTAINERD_ROOTLESS_ROOTLESSKIT_SLIRP4NETNS_SECCOMP:=auto}"
	net=$CONTAINERD_ROOTLESS_ROOTLESSKIT_NET
	mtu=$CONTAINERD_ROOTLESS_ROOTLESSKIT_MTU
	if [ -z $net ]; then
		if command -v slirp4netns >/dev/null 2>&1; then
			# If --netns-type is present in --help, slirp4netns is >= v0.4.0.
			if slirp4netns --help | grep -qw -- --netns-type; then
				net=slirp4netns
				if [ -z $mtu ]; then
					mtu=65520
				fi
			else
				echo "slirp4netns found but seems older than v0.4.0. Falling back to VPNKit."
			fi
		fi
		if [ -z $net ]; then
			if command -v vpnkit >/dev/null 2>&1; then
				net=vpnkit
			else
				echo "Either slirp4netns (>= v0.4.0) or vpnkit needs to be installed"
				exit 1
			fi
		fi
	fi
	if [ -z $mtu ]; then
		mtu=1500
	fi

	_CONTAINERD_ROOTLESS_CHILD=1
	export _CONTAINERD_ROOTLESS_CHILD

	# `selinuxenabled` always returns false in RootlessKit child, so we execute `selinuxenabled` in the parent.
	# https://github.com/rootless-containers/rootlesskit/issues/94
	if command -v selinuxenabled >/dev/null 2>&1; then
		if selinuxenabled; then
			_CONTAINERD_ROOTLESS_SELINUX=1
			export _CONTAINERD_ROOTLESS_SELINUX
		fi
	fi
	# Re-exec the script via RootlessKit, so as to create unprivileged {user,mount,network} namespaces.
	#
	# --copy-up allows removing/creating files in the directories by creating tmpfs and symlinks
	# * /etc:     copy-up is required so as to prevent `/etc/resolv.conf` in the
	#             namespace from being unexpectedly unmounted when `/etc/resolv.conf` is recreated on the host
	#             (by either systemd-networkd or NetworkManager)
	# * /run:     copy-up is required so that we can create /run/containerd (hardcoded) in our namespace
	# * /var/lib: copy-up is required so that we can create /var/lib/containerd in our namespace
	exec rootlesskit \
		--state-dir=$CONTAINERD_ROOTLESS_ROOTLESSKIT_STATE_DIR \
		--net=$net --mtu=$mtu \
		--slirp4netns-sandbox=$CONTAINERD_ROOTLESS_ROOTLESSKIT_SLIRP4NETNS_SANDBOX \
		--slirp4netns-seccomp=$CONTAINERD_ROOTLESS_ROOTLESSKIT_SLIRP4NETNS_SECCOMP \
		--disable-host-loopback --port-driver=$CONTAINERD_ROOTLESS_ROOTLESSKIT_PORT_DRIVER \
		--copy-up=/etc --copy-up=/run --copy-up=/var/lib \
		--propagation=rslave \
		$CONTAINERD_ROOTLESS_ROOTLESSKIT_FLAGS \
		$0 $@
else
	[ $_CONTAINERD_ROOTLESS_CHILD = 1 ]
	# Remove the *symlinks* for the existing files in the parent namespace if any,
	# so that we can create our own files in our mount namespace.
	# The actual files in the parent namespace are *not removed* by this rm command.
	rm -f /run/containerd /run/xtables.lock \
		/var/lib/containerd /var/lib/cni /etc/containerd

	# Bind-mount /etc/ssl.
	# Workaround for "x509: certificate signed by unknown authority" on openSUSE Tumbleweed.
	# https://github.com/rootless-containers/rootlesskit/issues/225
	realpath_etc_ssl=$(realpath /etc/ssl)
	rm -f /etc/ssl
	mkdir /etc/ssl
	mount --rbind "${realpath_etc_ssl}" /etc/ssl

	# Bind-mount /run/containerd
	mkdir -p "${XDG_RUNTIME_DIR}/containerd" "/run/containerd"
	mount --bind "${XDG_RUNTIME_DIR}/containerd" "/run/containerd"

	# Bind-mount /var/lib/containerd
	mkdir -p "${XDG_DATA_HOME}/containerd" "/var/lib/containerd"
	mount --bind "${XDG_DATA_HOME}/containerd" "/var/lib/containerd"

	# Bind-mount /var/lib/cni
	mkdir -p "${XDG_DATA_HOME}/cni" "/var/lib/cni"
	mount --bind "${XDG_DATA_HOME}/cni" "/var/lib/cni"

	# Bind-mount /etc/containerd
	mkdir -p "${XDG_CONFIG_HOME}/containerd" "/etc/containerd"
	mount --bind "${XDG_CONFIG_HOME}/containerd" "/etc/containerd"

	if [ -n "$_CONTAINERD_ROOTLESS_SELINUX" ]; then
		# iptables requires /run in the child to be relabeled. The actual /run in the parent is unaffected.
		# https://github.com/containers/podman/blob/e6fc34b71aa9d876b1218efe90e14f8b912b0603/libpod/networking_linux.go#L396-L401
		# https://github.com/moby/moby/issues/41230
		chcon system_u:object_r:iptables_var_run_t:s0 /run
	fi

	exec containerd $@
fi
