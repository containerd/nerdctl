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
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-iptables/iptables"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	iptablesutil "github.com/containerd/nerdctl/v2/pkg/testutil/iptables"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
)

func TestStopStart(t *testing.T) {
	const (
		hostPort = 8080
	)
	testContainerName := testutil.Identifier(t)
	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainerName).Run()

	base.Cmd("run", "-d",
		"--restart=no",
		"--name", testContainerName,
		"-p", fmt.Sprintf("127.0.0.1:%d:80", hostPort),
		testutil.NginxAlpineImage).AssertOK()

	check := func(httpGetRetry int) error {
		resp, err := nettestutil.HTTPGet(fmt.Sprintf("http://127.0.0.1:%d", hostPort), httpGetRetry, false)
		if err != nil {
			return err
		}
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if !strings.Contains(string(respBody), testutil.NginxAlpineIndexHTMLSnippet) {
			return fmt.Errorf("expected contain %q, got %q",
				testutil.NginxAlpineIndexHTMLSnippet, string(respBody))
		}
		return nil
	}

	assert.NilError(t, check(30))
	base.Cmd("stop", testContainerName).AssertOK()
	base.Cmd("exec", testContainerName, "ps").AssertFail()
	if check(1) == nil {
		t.Fatal("expected to get an error")
	}
	base.Cmd("start", testContainerName).AssertOK()
	assert.NilError(t, check(30))
}

func TestStopWithStopSignal(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		cmd := nerdtest.RunSigProxyContainer(nerdtest.SigQuit, false,
			[]string{"--stop-signal", nerdtest.SigQuit.String()}, data, helpers)
		helpers.Ensure("stop", data.Identifier())
		return cmd
	}

	// Verify that SIGQUIT was sent to the container AND that the container did forcefully exit
	testCase.Expected = test.Expects(137, nil, expect.Contains(nerdtest.SignalCaught))

	testCase.Run(t)
}

func TestStopCleanupForwards(t *testing.T) {
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

	base.Cmd("stop", testContainerName).AssertOK()
	assert.Equal(t, iptablesutil.ForwardExists(t, ipt, chain, containerIP, hostPort), false)
}

// Regression test for https://github.com/containerd/nerdctl/issues/3353
func TestStopCreated(t *testing.T) {
	t.Parallel()

	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)

	tearDown := func() {
		base.Cmd("rm", "-f", tID).Run()
	}

	setup := func() {
		base.Cmd("create", "--name", tID, testutil.CommonImage).AssertOK()
	}

	t.Cleanup(tearDown)
	tearDown()
	setup()

	base.Cmd("stop", tID).AssertOK()
}

func TestStopWithLongTimeoutAndSIGKILL(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	testContainerName := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", testContainerName).Run()

	// Start a container that sleeps forever
	base.Cmd("run", "-d", "--name", testContainerName, testutil.CommonImage, "sleep", "Inf").AssertOK()

	// Stop the container with a 5-second timeout and SIGKILL
	start := time.Now()
	base.Cmd("stop", "--time=5", "--signal", "SIGKILL", testContainerName).AssertOK()
	elapsed := time.Since(start)

	// The container should be stopped almost immediately, well before the 5-second timeout
	assert.Assert(t, elapsed < 5*time.Second, "Container wasn't stopped immediately with SIGKILL")
}

func TestStopWithTimeout(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	testContainerName := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", testContainerName).Run()

	// Start a container that sleeps forever
	base.Cmd("run", "-d", "--name", testContainerName, testutil.CommonImage, "sleep", "Inf").AssertOK()

	// Stop the container with a 3-second timeout
	start := time.Now()
	base.Cmd("stop", "--time=3", testContainerName).AssertOK()
	elapsed := time.Since(start)

	// The container should get the SIGKILL before the 10s default timeout
	assert.Assert(t, elapsed < 10*time.Second, "Container did not respect --timeout flag")
}
