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
# Forked from https://github.com/moby/moby/blob/v20.10.3/contrib/dockerd-rootless-setuptool.sh
# Copyright The Moby Authors.
# Licensed under the Apache License, Version 2.0
# NOTICE: https://github.com/moby/moby/blob/v20.10.3/NOTICE
# -----------------------------------------------------------------------------

# containerd-rootless-setuptool.sh: setup tool for containerd-rootless.sh
# Needs to be executed as a non-root user.
#
# Typical usage: containerd-rootless-setuptool.sh install
set -eu

# utility functions
INFO() {
	# https://github.com/koalaman/shellcheck/issues/1593
	# shellcheck disable=SC2039
	/bin/echo -e "\e[104m\e[97m[INFO]\e[49m\e[39m ${*}"
}

WARNING() {
	# shellcheck disable=SC2039
	/bin/echo >&2 -e "\e[101m\e[97m[WARNING]\e[49m\e[39m ${*}"
}

ERROR() {
	# shellcheck disable=SC2039
	/bin/echo >&2 -e "\e[101m\e[97m[ERROR]\e[49m\e[39m ${*}"
}

# constants
CONTAINERD_ROOTLESS_SH="containerd-rootless.sh"
SYSTEMD_CONTAINERD_UNIT="containerd.service"
SYSTEMD_BUILDKIT_UNIT="buildkit.service"
SYSTEMD_FUSE_OVERLAYFS_UNIT="containerd-fuse-overlayfs.service"
SYSTEMD_STARGZ_UNIT="stargz-snapshotter.service"
SYSTEMD_IPFS_UNIT="ipfs-daemon.service"
SYSTEMD_BYPASS4NETNSD_UNIT="bypass4netnsd.service"

# global vars
ARG0="$0"
REALPATH0="$(realpath "$ARG0")"
BIN=""
XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"
XDG_DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"

# run checks and also initialize global vars (BIN)
init() {
	id="$(id -u)"
	# User verification: deny running as root
	if [ "$id" = "0" ]; then
		ERROR "Refusing to install rootless containerd as the root user"
		exit 1
	fi

	# set BIN
	if ! BIN="$(command -v "$CONTAINERD_ROOTLESS_SH" 2>/dev/null)"; then
		ERROR "$CONTAINERD_ROOTLESS_SH needs to be present under \$PATH"
		exit 1
	fi
	BIN=$(dirname "$BIN")

	# detect systemd
	if ! systemctl --user show-environment >/dev/null 2>&1; then
		ERROR "Needs systemd (systemctl --user)"
		exit 1
	fi

	# HOME verification
	if [ -z "${HOME:-}" ] || [ ! -d "$HOME" ]; then
		ERROR "HOME needs to be set"
		exit 1
	fi
	if [ ! -w "$HOME" ]; then
		ERROR "HOME needs to be writable"
		exit 1
	fi

	# Validate XDG_RUNTIME_DIR
	if [ -z "${XDG_RUNTIME_DIR:-}" ] || [ ! -w "$XDG_RUNTIME_DIR" ]; then
		ERROR "Aborting because but XDG_RUNTIME_DIR (\"$XDG_RUNTIME_DIR\") is not set, does not exist, or is not writable"
		ERROR "Hint: this could happen if you changed users with 'su' or 'sudo'. To work around this:"
		ERROR "- try again by first running with root privileges 'loginctl enable-linger <user>' where <user> is the unprivileged user and export XDG_RUNTIME_DIR to the value of RuntimePath as shown by 'loginctl show-user <user>'"
		ERROR "- or simply log back in as the desired unprivileged user (ssh works for remote machines, machinectl shell works for local machines)"
		ERROR "See also https://rootlesscontaine.rs/getting-started/common/login/ ."
		exit 1
	fi
}

# CLI subcommand: "check"
cmd_entrypoint_check() {
	init
	INFO "Checking RootlessKit functionality"
	if ! rootlesskit \
		--net=slirp4netns \
		--disable-host-loopback \
		--copy-up=/etc --copy-up=/run --copy-up=/var/lib \
		true; then
		ERROR "RootlessKit failed, see the error messages and https://rootlesscontaine.rs/getting-started/common/ ."
		exit 1
	fi

	INFO "Checking cgroup v2"
	controllers="/sys/fs/cgroup/user.slice/user-${id}.slice/user@${id}.service/cgroup.controllers"
	if [ ! -f "${controllers}" ]; then
		WARNING "Enabling cgroup v2 is highly recommended, see https://rootlesscontaine.rs/getting-started/common/cgroup2/ "
	else
		for f in cpu memory pids; do
			if ! grep -qw "$f" "$controllers"; then
				WARNING "The cgroup v2 controller \"$f\" is not delegated for the current user (\"$controllers\"), see https://rootlesscontaine.rs/getting-started/common/cgroup2/"
			fi
		done
	fi

	INFO "Checking overlayfs"
	tmp=$(mktemp -d)
	mkdir -p "${tmp}/l" "${tmp}/u" "${tmp}/w" "${tmp}/m"
	if ! rootlesskit mount -t overlay -o lowerdir="${tmp}/l,upperdir=${tmp}/u,workdir=${tmp}/w" overlay "${tmp}/m"; then
		WARNING "Overlayfs is not enabled, consider installing fuse-overlayfs snapshotter (\`$0 install-fuse-overlayfs\`), " \
			"or see https://rootlesscontaine.rs/how-it-works/overlayfs/ to enable overlayfs."
	fi
	rm -rf "${tmp}"
	INFO "Requirements are satisfied"
}

# CLI subcommand: "nsenter"
cmd_entrypoint_nsenter() {
	# No need to call init()
	pid=$(cat "$XDG_RUNTIME_DIR/containerd-rootless/child_pid")
	exec nsenter --no-fork --wd="$(pwd)" --preserve-credentials -m -n -U -t "$pid" -- "$@"
}

show_systemd_error() {
	unit="$1"
	n="20"
	ERROR "Failed to start ${unit}. Run \`journalctl -n ${n} --no-pager --user --unit ${unit}\` to show the error log."
	ERROR "Before retrying installation, you might need to uninstall the current setup: \`$0 uninstall -f ; ${BIN}/rootlesskit rm -rf ${HOME}/.local/share/containerd\`"
}

install_systemd_unit() {
	unit="$1"
	unit_file="${XDG_CONFIG_HOME}/systemd/user/${unit}"
	if [ -f "${unit_file}" ]; then
		WARNING "File already exists, skipping: ${unit_file}"
	else
		INFO "Creating \"${unit_file}\""
		mkdir -p "${XDG_CONFIG_HOME}/systemd/user"
		cat >"${unit_file}"
		systemctl --user daemon-reload
	fi
	if ! systemctl --user --no-pager status "${unit}" >/dev/null 2>&1; then
		INFO "Starting systemd unit \"${unit}\""
		(
			set -x
			if ! systemctl --user start "${unit}"; then
				set +x
				show_systemd_error "${unit}"
				exit 1
			fi
			sleep 3
		)
	fi
	(
		set -x
		if ! systemctl --user --no-pager --full status "${unit}"; then
			set +x
			show_systemd_error "${unit}"
			exit 1
		fi
		systemctl --user enable "${unit}"
	)
	INFO "Installed \"${unit}\" successfully."
	INFO "To control \"${unit}\", run: \`systemctl --user (start|stop|restart) ${unit}\`"
}

uninstall_systemd_unit() {
	unit="$1"
	unit_file="${XDG_CONFIG_HOME}/systemd/user/${unit}"
	if [ ! -f "${unit_file}" ]; then
		INFO "Unit ${unit} is not installed"
		return
	fi
	(
		set -x
		systemctl --user stop "${unit}"
	) || :
	(
		set -x
		systemctl --user disable "${unit}"
	) || :
	rm -f "${unit_file}"
	INFO "Uninstalled \"${unit}\""
}

# CLI subcommand: "install"
cmd_entrypoint_install() {
	init
	cmd_entrypoint_check
	cat <<-EOT | install_systemd_unit "${SYSTEMD_CONTAINERD_UNIT}"
		[Unit]
		Description=containerd (Rootless)

		[Service]
		Environment=PATH=$BIN:/sbin:/usr/sbin:$PATH
		Environment=CONTAINERD_ROOTLESS_ROOTLESSKIT_FLAGS=${CONTAINERD_ROOTLESS_ROOTLESSKIT_FLAGS:-}
		ExecStart=$BIN/${CONTAINERD_ROOTLESS_SH}
		ExecReload=/bin/kill -s HUP \$MAINPID
		TimeoutSec=0
		RestartSec=2
		Restart=always
		StartLimitBurst=3
		StartLimitInterval=60s
		LimitNOFILE=infinity
		LimitNPROC=infinity
		LimitCORE=infinity
		TasksMax=infinity
		Delegate=yes
		Type=simple
		KillMode=mixed

		[Install]
		WantedBy=default.target
	EOT
	systemctl --user daemon-reload
	INFO "To run \"${SYSTEMD_CONTAINERD_UNIT}\" on system startup automatically, run: \`sudo loginctl enable-linger $(id -un)\`"
	INFO "------------------------------------------------------------------------------------------"
	INFO "Use \`nerdctl\` to connect to the rootless containerd."
	INFO "You do NOT need to specify \$CONTAINERD_ADDRESS explicitly."
}

# CLI subcommand: "install-buildkit"
cmd_entrypoint_install_buildkit() {
	init
	if ! command -v "buildkitd" >/dev/null 2>&1; then
		ERROR "buildkitd (https://github.com/moby/buildkit) needs to be present under \$PATH"
		exit 1
	fi
	if ! systemctl --user --no-pager status "${SYSTEMD_CONTAINERD_UNIT}" >/dev/null 2>&1; then
		ERROR "Install containerd first (\`$ARG0 install\`)"
		exit 1
	fi
	cat <<-EOT | install_systemd_unit "${SYSTEMD_BUILDKIT_UNIT}"
		[Unit]
		Description=BuildKit (Rootless)
		PartOf=${SYSTEMD_CONTAINERD_UNIT}

		[Service]
		Environment=PATH=$BIN:/sbin:/usr/sbin:$PATH
		ExecStart="$REALPATH0" nsenter buildkitd
		ExecReload=/bin/kill -s HUP \$MAINPID
		RestartSec=2
		Restart=always
		Type=simple
		KillMode=mixed

		[Install]
		WantedBy=default.target
	EOT
}

# CLI subcommand: "install-buildkit-containerd"
cmd_entrypoint_install_buildkit_containerd() {
	init
	if ! command -v "buildkitd" >/dev/null 2>&1; then
		ERROR "buildkitd (https://github.com/moby/buildkit) needs to be present under \$PATH"
		exit 1
	fi
	if [ ! -f "${XDG_CONFIG_HOME}/buildkit/buildkitd.toml" ]; then
		mkdir -p "${XDG_CONFIG_HOME}/buildkit"
		cat <<-EOF > "${XDG_CONFIG_HOME}/buildkit/buildkitd.toml"
			[worker.oci]
			enabled = false

			[worker.containerd]
			enabled = true
			rootless = true
		EOF
	fi
	if ! systemctl --user --no-pager status "${SYSTEMD_CONTAINERD_UNIT}" >/dev/null 2>&1; then
		ERROR "Install containerd first (\`$ARG0 install\`)"
		exit 1
	fi
	UNIT_NAME=${SYSTEMD_BUILDKIT_UNIT}
	BUILDKITD_FLAG=
	if [ -n "${CONTAINERD_NAMESPACE:-}" ] ; then
		UNIT_NAME="${CONTAINERD_NAMESPACE}-${SYSTEMD_BUILDKIT_UNIT}"
		BUILDKITD_FLAG="${BUILDKITD_FLAG} --addr=unix://${XDG_RUNTIME_DIR}/buildkit-${CONTAINERD_NAMESPACE}/buildkitd.sock --root=${XDG_DATA_HOME}/buildkit-${CONTAINERD_NAMESPACE} --containerd-worker-namespace=${CONTAINERD_NAMESPACE}"
	else
		WARNING "buildkitd has access to images in \"buildkit\" namespace by default. If you want to give buildkitd access to the images in \"default\" namespace, run this command with CONTAINERD_NAMESPACE=default"
	fi
	if [ -n "${CONTAINERD_SNAPSHOTTER:-}" ] ; then
		BUILDKITD_FLAG="${BUILDKITD_FLAG} --containerd-worker-snapshotter=${CONTAINERD_SNAPSHOTTER}"
	fi
	cat <<-EOT | install_systemd_unit "${UNIT_NAME}"
		[Unit]
		Description=BuildKit (Rootless)
		PartOf=${SYSTEMD_CONTAINERD_UNIT}

		[Service]
		Environment=PATH=$BIN:/sbin:/usr/sbin:$PATH
		ExecStart="$REALPATH0" nsenter -- buildkitd ${BUILDKITD_FLAG}
		ExecReload=/bin/kill -s HUP \$MAINPID
		RestartSec=2
		Restart=always
		Type=simple
		KillMode=mixed

		[Install]
		WantedBy=default.target
	EOT
}

# CLI subcommand: "install-bypass4netnsd"
cmd_entrypoint_install_bypass4netnsd() {
	init
	if ! command -v "bypass4netnsd" >/dev/null 2>&1; then
		ERROR "bypass4netnsd (https://github.com/rootless-containers/bypass4netns) needs to be present under \$PATH"
		exit 1
	fi
	command_v_bypass4netnsd="$(command -v bypass4netnsd)"
	# FIXME: bail if bypass4netnsd is an alias
	cat <<-EOT | install_systemd_unit "${SYSTEMD_BYPASS4NETNSD_UNIT}"
		[Unit]
		Description=bypass4netnsd (daemon for bypass4netns, accelerator for rootless containers)
		# Not PartOf=${SYSTEMD_CONTAINERD_UNIT}

		[Service]
		Environment=PATH=$BIN:/sbin:/usr/sbin:$PATH
		ExecStart="${command_v_bypass4netnsd}"
		ExecReload=/bin/kill -s HUP \$MAINPID
		RestartSec=2
		Restart=always
		Type=simple
		KillMode=mixed

		[Install]
		WantedBy=default.target
	EOT
	INFO "To use bypass4netnsd, set the \"nerdctl/bypass4netns=true\" label on containers, e.g., \`nerdctl run --label nerdctl/bypass4netns=true\`"
}

# CLI subcommand: "install-fuse-overlayfs"
cmd_entrypoint_install_fuse_overlayfs() {
	init
	if ! command -v "containerd-fuse-overlayfs-grpc" >/dev/null 2>&1; then
		ERROR "containerd-fuse-overlayfs-grpc (https://github.com/containerd/fuse-overlayfs-snapshotter) needs to be present under \$PATH"
		exit 1
	fi
	if ! command -v "fuse-overlayfs" >/dev/null 2>&1; then
		ERROR "fuse-overlayfs (https://github.com/containers/fuse-overlayfs) needs to be present under \$PATH"
		exit 1
	fi
	if ! systemctl --user --no-pager status "${SYSTEMD_CONTAINERD_UNIT}" >/dev/null 2>&1; then
		ERROR "Install containerd first (\`$ARG0 install\`)"
		exit 1
	fi
	cat <<-EOT | install_systemd_unit "${SYSTEMD_FUSE_OVERLAYFS_UNIT}"
		[Unit]
		Description=containerd-fuse-overlayfs (Rootless)
		PartOf=${SYSTEMD_CONTAINERD_UNIT}

		[Service]
		Environment=PATH=$BIN:/sbin:/usr/sbin:$PATH
		ExecStart="$REALPATH0" nsenter containerd-fuse-overlayfs-grpc "${XDG_RUNTIME_DIR}/containerd-fuse-overlayfs.sock" "${XDG_DATA_HOME}/containerd-fuse-overlayfs"
		ExecReload=/bin/kill -s HUP \$MAINPID
		RestartSec=2
		Restart=always
		Type=simple
		KillMode=mixed

		[Install]
		WantedBy=default.target
	EOT
	INFO "Add the following lines to \"${XDG_CONFIG_HOME}/containerd/config.toml\" manually, and then run \`systemctl --user restart ${SYSTEMD_CONTAINERD_UNIT}\`:"
	cat <<-EOT
		### BEGIN ###
		[proxy_plugins]
		  [proxy_plugins."fuse-overlayfs"]
		    type = "snapshot"
		    address = "${XDG_RUNTIME_DIR}/containerd-fuse-overlayfs.sock"
		###  END  ###
	EOT
	INFO "Set \`export CONTAINERD_SNAPSHOTTER=\"fuse-overlayfs\"\` to use the fuse-overlayfs snapshotter."
}

# CLI subcommand: "install-stargz"
cmd_entrypoint_install_stargz() {
	init
	if ! command -v "containerd-stargz-grpc" >/dev/null 2>&1; then
		ERROR "containerd-stargz-grpc (https://github.com/containerd/stargz-snapshotter) needs to be present under \$PATH"
		exit 1
	fi
	if ! systemctl --user --no-pager status "${SYSTEMD_CONTAINERD_UNIT}" >/dev/null 2>&1; then
		ERROR "Install containerd first (\`$ARG0 install\`)"
		exit 1
	fi
	if [ ! -f "${XDG_CONFIG_HOME}/containerd-stargz-grpc/config.toml" ]; then
		mkdir -p "${XDG_CONFIG_HOME}/containerd-stargz-grpc"
		touch "${XDG_CONFIG_HOME}/containerd-stargz-grpc/config.toml"
	fi
	cat <<-EOT | install_systemd_unit "${SYSTEMD_STARGZ_UNIT}"
		[Unit]
		Description=stargz snapshotter (Rootless)
		PartOf=${SYSTEMD_CONTAINERD_UNIT}

		[Service]
		Environment=PATH=$BIN:/sbin:/usr/sbin:$PATH
		Environment=IPFS_PATH=${XDG_DATA_HOME}/ipfs
		ExecStart="$REALPATH0" nsenter -- containerd-stargz-grpc -address "${XDG_RUNTIME_DIR}/containerd-stargz-grpc/containerd-stargz-grpc.sock" -root "${XDG_DATA_HOME}/containerd-stargz-grpc" -config "${XDG_CONFIG_HOME}/containerd-stargz-grpc/config.toml"
		ExecReload=/bin/kill -s HUP \$MAINPID
		RestartSec=2
		Restart=always
		Type=simple
		KillMode=mixed

		[Install]
		WantedBy=default.target
	EOT
	INFO "Add the following lines to \"${XDG_CONFIG_HOME}/containerd/config.toml\" manually, and then run \`systemctl --user restart ${SYSTEMD_CONTAINERD_UNIT}\`:"
	cat <<-EOT
		### BEGIN ###
		[proxy_plugins]
		  [proxy_plugins."stargz"]
		    type = "snapshot"
		    address = "${XDG_RUNTIME_DIR}/containerd-stargz-grpc/containerd-stargz-grpc.sock"
		###  END  ###
	EOT
	INFO "Set \`export CONTAINERD_SNAPSHOTTER=\"stargz\"\` to use the stargz snapshotter."
}

# CLI subcommand: "install-ipfs"
cmd_entrypoint_install_ipfs() {
	init
	if ! command -v "ipfs" >/dev/null 2>&1; then
		ERROR "ipfs needs to be present under \$PATH"
		exit 1
	fi
	if ! systemctl --user --no-pager status "${SYSTEMD_CONTAINERD_UNIT}" >/dev/null 2>&1; then
		ERROR "Install containerd first (\`$ARG0 install\`)"
		exit 1
	fi
	IPFS_PATH="${XDG_DATA_HOME}/ipfs"
	mkdir -p "${IPFS_PATH}"
	cat <<-EOT | install_systemd_unit "${SYSTEMD_IPFS_UNIT}"
		[Unit]
		Description=ipfs daemon for rootless nerdctl
		PartOf=${SYSTEMD_CONTAINERD_UNIT}

		[Service]
		Environment=PATH=$BIN:/sbin:/usr/sbin:$PATH
		Environment=IPFS_PATH=${IPFS_PATH}
		ExecStart="$REALPATH0" nsenter -- ipfs daemon $@
		ExecReload=/bin/kill -s HUP \$MAINPID
		RestartSec=2
		Restart=always
		Type=simple
		KillMode=mixed

		[Install]
		WantedBy=default.target
	EOT

	# Avoid using 5001(api)/8080(gateway) which are reserved by tests.
	# TODO: support unix socket
	systemctl --user stop "${SYSTEMD_IPFS_UNIT}"
	sleep 3
	IPFS_PATH=${IPFS_PATH} ipfs config Addresses.API "/ip4/127.0.0.1/tcp/5888"
	IPFS_PATH=${IPFS_PATH} ipfs config Addresses.Gateway "/ip4/127.0.0.1/tcp/5889"
	systemctl --user restart "${SYSTEMD_IPFS_UNIT}"
	sleep 3

	INFO "If you use stargz-snapshotter, add the following line to \"${XDG_CONFIG_HOME}/containerd-stargz-grpc/config.toml\" manually, and then run \`systemctl --user restart ${SYSTEMD_STARGZ_UNIT}\`:"
	cat <<-EOT
		### BEGIN ###
		ipfs = true
		###  END  ###
	EOT
	INFO "If you want to expose the port 4001 of ipfs daemon, re-install rootless containerd with CONTAINERD_ROOTLESS_ROOTLESSKIT_FLAGS=\"--publish=0.0.0.0:4001:4001/tcp\" environment variable."
	INFO "Set \`export IPFS_PATH=\"${IPFS_PATH}\"\` to use ipfs."
}

# CLI subcommand: "uninstall"
cmd_entrypoint_uninstall() {
	init
	uninstall_systemd_unit "${SYSTEMD_BUILDKIT_UNIT}"
	uninstall_systemd_unit "${SYSTEMD_FUSE_OVERLAYFS_UNIT}"
	uninstall_systemd_unit "${SYSTEMD_CONTAINERD_UNIT}"

	INFO "This uninstallation tool does NOT remove containerd binaries and data."
	INFO "To remove data, run: \`$BIN/rootlesskit rm -rf ${XDG_DATA_HOME}/containerd\`"
}

# CLI subcommand: "uninstall-buildkit"
cmd_entrypoint_uninstall_buildkit() {
	init
	uninstall_systemd_unit "${SYSTEMD_BUILDKIT_UNIT}"
	INFO "This uninstallation tool does NOT remove data."
	INFO "To remove data, run: \`$BIN/rootlesskit rm -rf ${XDG_DATA_HOME}/buildkit"
}

# CLI subcommand: "uninstall-buildkit-containerd"
cmd_entrypoint_uninstall_buildkit_containerd() {
	init
	UNIT_NAME=${SYSTEMD_BUILDKIT_UNIT}
	BUILDKIT_ROOT="${XDG_DATA_HOME}/buildkit"
	if [ -n "${CONTAINERD_NAMESPACE:-}" ] ; then
		UNIT_NAME="${CONTAINERD_NAMESPACE}-${SYSTEMD_BUILDKIT_UNIT}"
		BUILDKIT_ROOT="${XDG_DATA_HOME}/buildkit-${CONTAINERD_NAMESPACE}"
	fi
	uninstall_systemd_unit "${UNIT_NAME}"
	INFO "This uninstallation tool does NOT remove data."
	INFO "To remove data, run: \`$BIN/rootlesskit rm -rf ${BUILDKIT_ROOT}\`"
}

# CLI subcommand: "uninstall-bypass4netnsd"
cmd_entrypoint_uninstall_bypass4netnsd() {
	init
	uninstall_systemd_unit "${SYSTEMD_BYPASS4NETNSD_UNIT}"
}

# CLI subcommand: "uninstall-fuse-overlayfs"
cmd_entrypoint_uninstall_fuse_overlayfs() {
	init
	uninstall_systemd_unit "${SYSTEMD_FUSE_OVERLAYFS_UNIT}"
	INFO "This uninstallation tool does NOT remove data."
	INFO "To remove data, run: \`$BIN/rootlesskit rm -rf ${XDG_DATA_HOME}/containerd-fuse-overlayfs"
}

# CLI subcommand: "uninstall-stargz"
cmd_entrypoint_uninstall_stargz() {
	init
	uninstall_systemd_unit "${SYSTEMD_STARGZ_UNIT}"
	INFO "This uninstallation tool does NOT remove data."
	INFO "To remove data, run: \`$BIN/rootlesskit rm -rf ${XDG_DATA_HOME}/containerd-stargz-grpc"
}

# CLI subcommand: "uninstall-ipfs"
cmd_entrypoint_uninstall_ipfs() {
	init
	uninstall_systemd_unit "${SYSTEMD_IPFS_UNIT}"
	INFO "This uninstallation tool does NOT remove data."
	INFO "To remove data, run: \`$BIN/rootlesskit rm -rf ${XDG_DATA_HOME}/ipfs"
}

# text for --help
usage() {
	echo "Usage: ${ARG0} [OPTIONS] COMMAND"
	echo
	echo "A setup tool for Rootless containerd (${CONTAINERD_ROOTLESS_SH})."
	echo
	echo "Commands:"
	echo "  check        Check prerequisites"
	echo "  nsenter      Enter into RootlessKit namespaces (mostly for debugging)"
	echo "  install      Install systemd unit and show how to manage the service"
	echo "  uninstall    Uninstall systemd unit"
	echo
	echo "Add-on commands (BuildKit):"
	echo "  install-buildkit            Install the systemd unit for BuildKit"
	echo "  uninstall-buildkit          Uninstall the systemd unit for BuildKit"
	echo
	echo "Add-on commands (bypass4netnsd):"
	echo "  install-bypass4netnsd       Install the systemd unit for bypass4netnsd"
	echo "  uninstall-bypass4netnsd     Uninstall the systemd unit for bypass4netnsd"
	echo
	echo "Add-on commands (fuse-overlayfs):"
	echo "  install-fuse-overlayfs      Install the systemd unit for fuse-overlayfs snapshotter"
	echo "  uninstall-fuse-overlayfs    Uninstall the systemd unit for fuse-overlayfs snapshotter"
	echo
	echo "Add-on commands (stargz):"
	echo "  install-stargz              Install the systemd unit for stargz snapshotter"
	echo "  uninstall-stargz            Uninstall the systemd unit for stargz snapshotter"
	echo
	echo "Add-on commands (ipfs):"
	echo "  install-ipfs [ipfs-daemon-flags...]  Install the systemd unit for ipfs daemon. Specify \"--offline\" if run the daemon in offline mode. Specify \"--init\" to initialize IPFS repository as well."
	echo "  uninstall-ipfs                       Uninstall the systemd unit for ipfs daemon"
	echo
	echo "Add-on commands (BuildKit containerd worker):"
	echo "  install-buildkit-containerd   Install the systemd unit for BuildKit with CONTAINERD_NAMESPACE=${CONTAINERD_NAMESPACE:-} and CONTAINERD_SNAPSHOTTER=${CONTAINERD_SNAPSHOTTER:-}"
	echo "  uninstall-buildkit-containerd Uninstall the systemd unit for BuildKit with CONTAINERD_NAMESPACE=${CONTAINERD_NAMESPACE:-} and CONTAINERD_SNAPSHOTTER=${CONTAINERD_SNAPSHOTTER:-}"
}

# parse CLI args
if ! args="$(getopt -o h --long help -n "$ARG0" -- "$@")"; then
	usage
	exit 1
fi
eval set -- "$args"
while [ "$#" -gt 0 ]; do
	arg="$1"
	shift
	case "$arg" in
	-h | --help)
		usage
		exit 0
		;;
	--)
		break
		;;
	*)
		# XXX this means we missed something in our "getopt" arguments above!
		ERROR "Scripting error, unknown argument '$arg' when parsing script arguments."
		exit 1
		;;
	esac
done

command=$(echo "${1:-}" | sed -e "s/-/_/g")
if [ -z "$command" ]; then
	ERROR "No command was specified. Run with --help to see the usage. Maybe you want to run \`$ARG0 install\`?"
	exit 1
fi

if ! command -v "cmd_entrypoint_${command}" >/dev/null 2>&1; then
	ERROR "Unknown command: ${command}. Run with --help to see the usage."
	exit 1
fi

# main
shift
"cmd_entrypoint_${command}" "$@"
