#!/bin/sh
# Copyright (C) containerd Authors.
# Copyright (C) nerdctl Authors.
# Copyright (C) Moby Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Forked from https://github.com/moby/moby/blob/v20.10.3/contrib/dockerd-rootless-setuptool.sh

# -----------------------------------------------------------------------------

# containerd-rootless-setuptool.sh: setup tool for containerd-rootless.sh
# Needs to be executed as a non-root user.
#
# Typical usage: containerd-rootless-setuptool.sh install
set -eu

# utility functions
INFO() {
	/bin/echo -e "\e[104m\e[97m[INFO]\e[49m\e[39m $@"
}

WARNING() {
	/bin/echo >&2 -e "\e[101m\e[97m[WARNING]\e[49m\e[39m $@"
}

ERROR() {
	/bin/echo >&2 -e "\e[101m\e[97m[ERROR]\e[49m\e[39m $@"
}

# constants
CONTAINERD_ROOTLESS_SH="containerd-rootless.sh"
SYSTEMD_UNIT="containerd.service"

# global vars
ARG0="$0"
BIN=""
CFG_DIR=""

# run checks and also initialize global vars (BIN, CFG_DIR)
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

	# set CFG_DIR
	CFG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}"

	# Validate XDG_RUNTIME_DIR
	if [ -z "${XDG_RUNTIME_DIR:-}" ] || [ ! -w "$XDG_RUNTIME_DIR" ]; then
		ERROR "Aborting because but XDG_RUNTIME_DIR (\"$XDG_RUNTIME_DIR\") is not set, does not exist, or is not writable"
		ERROR "Hint: this could happen if you changed users with 'su' or 'sudo'. To work around this:"
		ERROR "- try again by first running with root privileges 'loginctl enable-linger <user>' where <user> is the unprivileged user and export XDG_RUNTIME_DIR to the value of RuntimePath as shown by 'loginctl show-user <user>'"
		ERROR "- or simply log back in as the desired unprivileged user (ssh works for remote machines, machinectl shell works for local machines)"
		exit 1
	fi

	INFO "Checking RootlessKit functionality"
	if ! rootlesskit \
		--net=slirp4netns \
		--disable-host-loopback \
		--port-driver=builtin \
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
	mkdir -p $tmp/l $tmp/u $tmp/w $tmp/m
	if ! rootlesskit mount -t overlay -o lowerdir=$tmp/l,upperdir=$tmp/u,workdir=$tmp/w overlay $tmp/m; then
		WARNING "Overlayfs is not enabled, see https://rootlesscontaine.rs/how-it-works/overlayfs/"
		WARNING "(TL;DR: Use Ubuntu, Debian, or kernel >= 5.11 .)"
	fi
	rm -rf $tmp
}

# CLI subcommand: "check"
cmd_entrypoint_check() {
	# requirements are checked in init()
	init
	INFO "Requirements are satisfied"
}

# CLI subcommand: "nsenter"
cmd_entrypoint_nsenter() {
	# No need to call init()
	pid=$(cat "$XDG_RUNTIME_DIR/containerd-rootless/child_pid")
	exec nsenter --no-fork --wd="$(pwd)" --preserve-credentials -m -n -U -t "$pid" -- "$@"
}

show_systemd_error() {
	n="20"
	ERROR "Failed to start ${SYSTEMD_UNIT}. Run \`journalctl -n ${n} --no-pager --user --unit ${SYSTEMD_UNIT}\` to show the error log."
	ERROR "Before retrying installation, you might need to uninstall the current setup: \`$0 uninstall -f ; ${BIN}/rootlesskit rm -rf ${HOME}/.local/share/containerd\`"
}

# install (systemd)
install_systemd() {
	mkdir -p "${CFG_DIR}/systemd/user"
	unit_file="${CFG_DIR}/systemd/user/${SYSTEMD_UNIT}"
	if [ -f "${unit_file}" ]; then
		WARNING "File already exists, skipping: ${unit_file}"
	else
		INFO "Creating ${unit_file}"
		cat <<-EOT >"${unit_file}"
			[Unit]
			Description=containerd (Rootless)

			[Service]
			Environment=PATH=$BIN:/sbin:/usr/sbin:$PATH
			ExecStart=$BIN/containerd-rootless.sh
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
	fi
	if ! systemctl --user --no-pager status "${SYSTEMD_UNIT}" >/dev/null 2>&1; then
		INFO "starting systemd service ${SYSTEMD_UNIT}"
		(
			set -x
			if ! systemctl --user start "${SYSTEMD_UNIT}"; then
				set +x
				show_systemd_error
				exit 1
			fi
			sleep 3
		)
	fi
	(
		set -x
		if ! systemctl --user --no-pager --full status "${SYSTEMD_UNIT}"; then
			set +x
			show_systemd_error
			exit 1
		fi
		systemctl --user enable "${SYSTEMD_UNIT}"
	)
	INFO "Installed ${SYSTEMD_UNIT} successfully."
	INFO "To control ${SYSTEMD_UNIT}, run: \`systemctl --user (start|stop|restart) ${SYSTEMD_UNIT}\`"
	INFO "To run ${SYSTEMD_UNIT} on system startup, run: \`sudo loginctl enable-linger $(id -un)\`"
	echo
	INFO "Use \`nerdctl\` to connect to the rootless containerd."
	INFO "You do NOT need to specify \$CONTAINERD_ADDRESS explicitly."
}

# CLI subcommand: "install"
cmd_entrypoint_install() {
	# requirements are checked in init()
	init
	install_systemd
}

# CLI subcommand: "uninstall"
cmd_entrypoint_uninstall() {
	# requirements are checked in init()
	init
	unit_file="${CFG_DIR}/systemd/user/${SYSTEMD_UNIT}"
	(
		set -x
		systemctl --user stop "${SYSTEMD_UNIT}"
	) || :
	(
		set -x
		systemctl --user disable "${SYSTEMD_UNIT}"
	) || :
	rm -f "${unit_file}"
	INFO "Uninstalled ${SYSTEMD_UNIT}"

	INFO "This uninstallation tool does NOT remove containerd binaries and data."
	INFO "To remove data, run: \`$BIN/rootlesskit rm -rf $HOME/.local/share/containerd\`"
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

command="${1:-}"
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
