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

package main

import (
	"os/exec"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestRunHostProcessContainer(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	hostname, err := exec.Command("hostname").Output()
	if err != nil {
		t.Fatalf("unable to get hostname: %s", err)
	}

	base.Cmd("run", "--rm", "--isolation=host", testutil.WindowsNano, "hostname").AssertOutContains(string(hostname))
	output := base.Cmd("run", "--rm", "--isolation=host", testutil.WindowsNano, "whoami").Out()
	t.Logf("whoami %s", output)
}

func TestRunHostProcessContainerAsUser(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)

	hostuser := "nt authority\\system"
	base.Cmd("run", "--rm", "--isolation=host", "-u", "NT AUTHORITY\\SYSTEM", testutil.WindowsNano, "whoami").AssertOutContains(hostuser)
}

func TestRunHostProcessContainerAsService(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	hostuser := "nt authority\\local service"
	base.Cmd("run", "--rm", "--isolation=host", "-u", "NT AUTHORITY\\Local Service", testutil.WindowsNano, "whoami").AssertOutContains(hostuser)
}

func TestRunHostProcessContainerAslocalService(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	hostuser := "nt authority\\network service"
	base.Cmd("run", "--rm", "--isolation=host", "-u", "NT AUTHORITY\\Network Service", testutil.WindowsNano, "whoami").AssertOutContains(hostuser)
}

func TestRunProcessIsolated(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)

	containerUser := "ContainerUser"
	base.Cmd("run", "--rm", "--isolation=process", "-u", containerUser, testutil.WindowsNano, "whoami").AssertOutContains(containerUser)
}
