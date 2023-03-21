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
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"golang.org/x/sys/windows/svc/mgr"
)

func TestTopProcessContainer(t *testing.T) {
	testContainerName := testutil.Identifier(t)

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainerName).Run()

	base.Cmd("run", "-d", "--name", testContainerName, testutil.WindowsNano, "sleep", "5").AssertOK()
	base.Cmd("top", testContainerName).AssertOK()
}

func TestTopHyperVContainer(t *testing.T) {
	// Hyper-V Virtual Machine Management service
	hypervServiceName := "vmms"

	m, err := mgr.Connect()
	if err != nil {
		t.Fatalf("unable to access Windows service control manager: %s", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(hypervServiceName)
	// hyperv service is not present, hyperv is not enabled
	if err != nil {
		t.Skip("HyperV is not enabled, skipping test")
	}
	defer s.Close()

	testContainerName := testutil.Identifier(t)

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainerName).Run()

	base.Cmd("run", "--isolation", "hyperv", "-d", "--name", testContainerName, testutil.WindowsNano, "sleep", "5").AssertOK()
	base.Cmd("top", testContainerName).AssertOK()
}
