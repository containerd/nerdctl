/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package container

import (
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestRunWithSystemdAlways(t *testing.T) {
	testutil.DockerIncompatible(t)
	t.Parallel()
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)
	defer base.Cmd("container", "rm", "-f", containerName).AssertOK()

	base.Cmd("run", "--name", containerName, "--systemd=always", "--entrypoint=/bin/bash", testutil.UbuntuImage, "-c", "mount | grep cgroup").AssertOutContains("(rw,")

	base.Cmd("inspect", "--format", "{{json .Config.Labels}}", containerName).AssertOutContains("SIGRTMIN+3")

}

func TestRunWithSystemdTrueEnabled(t *testing.T) {
	testutil.DockerIncompatible(t)
	t.Parallel()
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)
	defer base.Cmd("container", "rm", "-f", containerName).AssertOK()

	base.Cmd("run", "-d", "--name", containerName, "--systemd=true", "--entrypoint=/sbin/init", testutil.SystemdImage).AssertOK()

	base.Cmd("inspect", "--format", "{{json .Config.Labels}}", containerName).AssertOutContains("SIGRTMIN+3")

	base.Cmd("exec", containerName, "sh", "-c", "--", `tries=0
until systemctl is-system-running >/dev/null 2>&1; do
	>&2 printf "Waiting for systemd to come up...\n"
	sleep 1s
	tries=$(( tries + 1))
	[ $tries -lt 10 ] || {
		>&2 printf "systemd failed to come up in a reasonable amount of time\n"
		exit 1
	}
done
systemctl list-jobs`).AssertOutContains("jobs")
}

func TestRunWithSystemdTrueDisabled(t *testing.T) {
	testutil.DockerIncompatible(t)
	t.Parallel()
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", containerName).AssertOK()

	base.Cmd("run", "--name", containerName, "--systemd=true", "--entrypoint=/bin/bash", testutil.SystemdImage, "-c", "systemctl list-jobs || true").AssertCombinedOutContains("System has not been booted with systemd as init system")
}

func TestRunWithSystemdFalse(t *testing.T) {
	testutil.DockerIncompatible(t)
	t.Parallel()
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", containerName).AssertOK()

	base.Cmd("run", "--name", containerName, "--systemd=false", "--entrypoint=/bin/bash", testutil.UbuntuImage, "-c", "mount | grep cgroup").AssertOutContains("(ro,")

	base.Cmd("inspect", "--format", "{{json .Config.Labels}}", containerName).AssertOutContains("SIGTERM")
}

func TestRunWithNoSystemd(t *testing.T) {
	testutil.DockerIncompatible(t)
	t.Parallel()
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", containerName).AssertOK()

	base.Cmd("run", "--name", containerName, "--entrypoint=/bin/bash", testutil.UbuntuImage, "-c", "mount | grep cgroup").AssertOutContains("(ro,")

	base.Cmd("inspect", "--format", "{{json .Config.Labels}}", containerName).AssertOutContains("SIGTERM")
}

func TestRunWithSystemdPrivilegedError(t *testing.T) {
	testutil.DockerIncompatible(t)
	t.Parallel()
	base := testutil.NewBase(t)

	base.Cmd("run", "--privileged", "--rm", "--systemd=always", "--entrypoint=/sbin/init", testutil.SystemdImage).AssertCombinedOutContains("if --privileged is used with systemd `--security-opt privileged-without-host-devices` must also be used")
}

func TestRunWithSystemdPrivilegedSuccess(t *testing.T) {
	testutil.DockerIncompatible(t)
	t.Parallel()
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)
	defer base.Cmd("container", "rm", "-f", containerName).AssertOK()

	base.Cmd("run", "-d", "--name", containerName, "--privileged", "--security-opt", "privileged-without-host-devices", "--systemd=true", "--entrypoint=/sbin/init", testutil.SystemdImage).AssertOK()

	base.Cmd("inspect", "--format", "{{json .Config.Labels}}", containerName).AssertOutContains("SIGRTMIN+3")
}
