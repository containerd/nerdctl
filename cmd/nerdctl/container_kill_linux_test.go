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
	"fmt"
	"strings"
	"testing"

	"github.com/coreos/go-iptables/iptables"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	iptablesutil "github.com/containerd/nerdctl/v2/pkg/testutil/iptables"
)

// TestKillCleanupForwards runs a container that exposes a port and then kill it.
// The test checks that the kill command effectively clean up
// the iptables forwards creted from the run.
func TestKillCleanupForwards(t *testing.T) {
	const (
		hostPort          = 9999
		testContainerName = "ngx"
	)
	base := testutil.NewBase(t)
	defer func() {
		base.Cmd("rm", "-f", testContainerName).Run()
	}()

	// skip if rootless
	if rootlessutil.IsRootless() {
		t.Skip("pkg/testutil/iptables does not support rootless")
	}

	ipt, err := iptables.New()
	assert.NilError(t, err)

	containerID := base.Cmd("run", "-d",
		"--restart=no",
		"--name", testContainerName,
		"-p", fmt.Sprintf("127.0.0.1:%d:80", hostPort),
		testutil.NginxAlpineImage).Run().Stdout()
	containerID = strings.TrimSuffix(containerID, "\n")

	containerIP := base.Cmd("inspect",
		"-f",
		"'{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}'",
		testContainerName).Run().Stdout()
	containerIP = strings.ReplaceAll(containerIP, "'", "")
	containerIP = strings.TrimSuffix(containerIP, "\n")

	// define iptables chain name depending on the target (docker/nerdctl)
	var chain string
	if testutil.GetTarget() == testutil.Docker {
		chain = "DOCKER"
	} else {
		redirectChain := "CNI-HOSTPORT-DNAT"
		chain = iptablesutil.GetRedirectedChain(t, ipt, redirectChain, testutil.Namespace, containerID)
	}
	assert.Equal(t, iptablesutil.ForwardExists(t, ipt, chain, containerIP, hostPort), true)

	base.Cmd("kill", testContainerName).AssertOK()
	assert.Equal(t, iptablesutil.ForwardExists(t, ipt, chain, containerIP, hostPort), false)
}
