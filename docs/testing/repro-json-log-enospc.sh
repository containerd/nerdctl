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

# Linux/root only. Mounts a private 8 MiB tmpfs; never fills the host disk.
set -euo pipefail
nerdctl=${1:-nerdctl}
if [[ $(uname -s) != Linux || $EUID != 0 ]]; then
    echo 'Run as root on a disposable Linux host with containerd running.' >&2
    exit 2
fi
work=$(mktemp -d /tmp/nerdctl-log-enospc.XXXXXX)
namespace="log-enospc-${work##*.}"
mounted=false
# Invoked by the EXIT trap.
# shellcheck disable=SC2329
cleanup() {
    "$nerdctl" --namespace "$namespace" --snapshotter "${SNAPSHOTTER:-overlayfs}" rm -f writer >/dev/null 2>&1 || true
    if $mounted; then umount "$work/logs"; fi
    rm -rf "$work"
}
trap cleanup EXIT
mkdir "$work/logs" "$work/control"
mount -t tmpfs -o size=8m tmpfs "$work/logs"
mounted=true
"$nerdctl" --version
# Expand the workload variables inside the container, not in this shell.
# shellcheck disable=SC2016
"$nerdctl" --namespace "$namespace" --snapshotter "${SNAPSHOTTER:-overlayfs}" run -d \
    --name writer --net none --log-driver json-file \
    --log-opt "log-path=$work/logs/output.log" --log-opt max-size=1m --log-opt max-file=2 \
    -v "$work/control:/control" docker.io/library/alpine:3.22 sh -c '
    echo READY
    touch /control/ready
    while [ ! -e /control/flood ]; do sleep 0.05; done
    payload=$(head -c 1024 /dev/zero | tr "\000" x)
    i=0
    while [ "$i" -lt 30000 ]; do
        printf "%s\n" "$payload"
        printf "%s\n" "$payload" >&2
        i=$((i + 1))
        printf "%s\n" "$i" > /control/progress
    done
    touch /control/flood-done
    while [ ! -e /control/recover ]; do sleep 0.05; done
    echo RECOVERED_STDOUT
    echo RECOVERED_STDERR >&2
    touch /control/done
    sleep 300
' >/dev/null
wait_file() {
    local file=$1
    for ((attempt=0; attempt<200; attempt++)); do
        if [[ -e $file ]]; then return 0; fi
        sleep 0.1
    done
    return 1
}
wait_file "$work/control/ready"
# The expected dd failure must specifically be ENOSPC, not an unrelated error.
if LC_ALL=C dd if=/dev/zero of="$work/logs/filler" bs=1M count=9 2>"$work/fill-error"; then
    echo 'Filesystem did not fill' >&2
    exit 2
fi
grep -q 'No space left on device' "$work/fill-error"
echo 'Injected ENOSPC on private 8 MiB log filesystem'
touch "$work/control/flood"
sleep 3
printf 'Progress before freeing space: '
cat "$work/control/progress"
rm "$work/logs/filler"
touch "$work/control/recover"
if ! wait_file "$work/control/done"; then
    printf 'FAIL: container output stayed blocked after freeing space; progress='
    cat "$work/control/progress"
    exit 1
fi
# The control file is written after the two echo calls, but logging may be async.
for ((attempt=0; attempt<100; attempt++)); do
    if grep -q RECOVERED_STDOUT "$work/logs/output.log" && \
       grep -q RECOVERED_STDERR "$work/logs/output.log"; then
        echo 'PASS: container completed 30000 writes per stream and both logs recovered'
        exit 0
    fi
    sleep 0.1
done
echo 'FAIL: container progressed, but JSON logging did not recover after freeing space'
exit 1
