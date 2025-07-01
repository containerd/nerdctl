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

# shellcheck disable=SC2034,SC2015
set -o errexit -o errtrace -o functrace -o nounset -o pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]:-$PWD}")" 2>/dev/null 1>&2 && pwd)"
readonly root

# If no argument is provided, run both flaky and not-flaky test suites.
if [ "$#" == 0 ]; then
  "$root"/integration.sh -test.only-flaky=false
  "$root"/integration.sh -test.only-flaky=true
  exit
fi

##### Import helper libraries
# shellcheck source=/dev/null
. "$root"/../mod/wax/scripts/collector.sh

##### Configuration
# Where to store report files
readonly report_location="${WAX_REPORT_LOCATION:-$HOME/nerdctl-test-report}"
# Where to store gotestsum log file
readonly gotestsum_log_main="$report_location"/test-integration.log
readonly gotestsum_log_flaky="$report_location"/test-integration-flaky.log
# Total run timeout
readonly timeout="60m"
# Number of retries for flaky tests
readonly retries="2"
readonly need_sudo="${WITH_SUDO:-}"

##### Prepare gotestsum arguments
mkdir -p "$report_location"
# Format and packages to test
args=(--format=testname --packages="$root"/../cmd/nerdctl/...)
# Log file
gotestsum_log="$gotestsum_log_main"
for arg in "$@"; do
  if [ "$arg" == "-test.only-flaky=true" ] || [ "$arg" == "-test.only-flaky" ]; then
    args+=("--rerun-fails=$retries")
    gotestsum_log="$gotestsum_log_flaky"
    break
  fi
done
args+=(--jsonfile "$gotestsum_log" --)

##### Append go test arguments
# Honor sudo
[ "$need_sudo" != true ] && [ "$need_sudo" != yes ] && [ "$need_sudo" != 1 ] || args+=(-exec sudo)
# About `-p 1`, see https://github.com/containerd/nerdctl/blob/main/docs/testing/README.md#about-parallelization
args+=(-timeout="$timeout" -p 1 -args -test.allow-kill-daemon "$@")

# FIXME: this should not be the responsibility of the test script
# Instead, it should be in the Dockerfile (or other stack provisioning script) - eg: /etc/systemd/system/securityfs.service
# [Unit]
# Description=Kernel Security File System
# DefaultDependencies=no
# Before=sysinit.target
# Before=apparmor.service
# ConditionSecurity=apparmor
# ConditionPathIsMountPoint=!/sys/kernel/security
#
# [Service]
# Type=oneshot
# ExecStart=/bin/mount -t securityfs -o nosuid,nodev,noexec securityfs /sys/kernel/security
#
# [Install]
# WantedBy=sysinit.target
if [[ "$(id -u)" = "0" ]]; then
  # Ensure securityfs is mounted for apparmor to work
  if ! mountpoint -q /sys/kernel/security; then
    mount -tsecurityfs securityfs /sys/kernel/security
  fi
fi

##### Run it
ex=0
gotestsum "${args[@]}" || ex=$?

##### Post: collect logs into the report location
collect::logs "$report_location"

# Honor gotestsum exit code
exit "$ex"