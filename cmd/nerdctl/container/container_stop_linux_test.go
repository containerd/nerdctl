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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-iptables/iptables"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	iptablesutil "github.com/containerd/nerdctl/v2/pkg/testutil/iptables"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/portlock"
)

func TestStopStart(t *testing.T) {
	testCase := nerdtest.Setup()

	httpCheck := func(data test.Data, httpGetRetry int) error {
		hostPort, _ := strconv.Atoi(data.Labels().Get("hostPort"))
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

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
		port, err := strconv.Atoi(data.Labels().Get("hostPort"))
		if err == nil {
			portlock.Release(port)
		}
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		port, err := portlock.Acquire(0)
		if err != nil {
			helpers.T().Log(fmt.Sprintf("failed to acquire port: %v", err))
			helpers.T().FailNow()
		}
		data.Labels().Set("hostPort", strconv.Itoa(port))
		data.Labels().Set("containerName", data.Identifier())
		helpers.Ensure("run", "-d",
			"--restart=no",
			"--name", data.Identifier(),
			"-p", fmt.Sprintf("127.0.0.1:%d:80", port),
			testutil.NginxAlpineImage)

		assert.NilError(helpers.T(), httpCheck(data, 5))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "container is stopped",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("stop", data.Labels().Get("containerName"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						helpers.Fail("exec", data.Labels().Get("containerName"), "ps")
						assert.Assert(t, httpCheck(data, 1) != nil, "expected HTTP to fail after stop")
					},
				}
			},
		},
		{
			Description: "container is restarted",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("start", data.Labels().Get("containerName"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						assert.NilError(t, httpCheck(data, 5))
					},
				}
			},
		},
	}

	testCase.Run(t)
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
	testCase.Expected = test.Expects(expect.ExitCodeSigkill, nil, expect.Contains(nerdtest.SignalCaught))

	testCase.Run(t)
}

func TestStopCleanupForwards(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Rootless)

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
		port, err := strconv.Atoi(data.Labels().Get("hostPort"))
		if err == nil {
			portlock.Release(port)
		}
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		port, err := portlock.Acquire(0)
		if err != nil {
			helpers.T().Log(fmt.Sprintf("failed to acquire port: %v", err))
			helpers.T().FailNow()
		}
		data.Labels().Set("hostPort", strconv.Itoa(port))

		containerID := strings.TrimSpace(helpers.Capture("run", "-d",
			"--restart=no",
			"--name", data.Identifier(),
			"-p", fmt.Sprintf("127.0.0.1:%d:80", port),
			testutil.NginxAlpineImage))

		containerIP := strings.TrimSpace(helpers.Capture("inspect",
			"-f", "{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}",
			data.Identifier()))

		data.Labels().Set("containerIP", containerIP)

		ipt, err := iptables.New()
		assert.NilError(helpers.T(), err)

		// define iptables chain name depending on the target (docker/nerdctl)
		var chain string
		if nerdtest.IsDocker() {
			chain = "DOCKER"
		} else {
			chain = iptablesutil.GetRedirectedChain(t, ipt, "CNI-HOSTPORT-DNAT", testutil.Namespace, containerID)
		}
		data.Labels().Set("chain", chain)

		assert.Equal(helpers.T(), iptablesutil.ForwardExists(t, ipt, chain, containerIP, port), true)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("stop", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, _ tig.T) {
				ipt, err := iptables.New()
				assert.NilError(t, err)
				chain := data.Labels().Get("chain")
				containerIP := data.Labels().Get("containerIP")
				port, _ := strconv.Atoi(data.Labels().Get("hostPort"))
				assert.Equal(t, iptablesutil.ForwardExists(t, ipt, chain, containerIP, port), false)
			},
		}
	}

	testCase.Run(t)
}

// Regression test for https://github.com/containerd/nerdctl/issues/3353
func TestStopCreated(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("create", "--name", data.Identifier(), testutil.CommonImage)
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("stop", data.Identifier())
	}
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, nil)
	testCase.Run(t)
}

func TestStopWithLongTimeoutAndSIGKILL(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// Start a container that sleeps forever
		helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// Stop the container with a 5-second timeout and SIGKILL
		// The container should be stopped almost immediately, well before the 5-second timeout
		cmd := helpers.Command("stop", "--time=5", "--signal", "SIGKILL", data.Identifier())
		cmd.WithTimeout(5 * time.Second)
		return cmd
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, nil)

	testCase.Run(t)
}

func TestStopWithTimeout(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// Start a container that sleeps forever
		helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// Stop the container with a 3-second timeout
		// The container should get the SIGKILL before the 10s default timeout
		cmd := helpers.Command("stop", "--time=3", data.Identifier())
		cmd.WithTimeout(10 * time.Second)
		return cmd
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, nil)

	testCase.Run(t)
}
func TestStopCleanupFIFOs(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.NoParallel = true
	testCase.Require = require.All(
		require.Not(nerdtest.Rootless),
		require.Not(nerdtest.Docker),
	)

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		oldNumFifos, err := countFIFOFiles("/run/containerd/fifo/")
		assert.NilError(helpers.T(), err)
		data.Labels().Set("oldNumFifos", strconv.Itoa(oldNumFifos))

		cmd := helpers.Command("run", "--rm", "--name", data.Identifier(), testutil.NginxAlpineImage)
		cmd.Background()

		time.Sleep(2 * time.Second)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("stop", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, _ tig.T) {
				oldNumFifos, _ := strconv.Atoi(data.Labels().Get("oldNumFifos"))
				newNumFifos, err := countFIFOFiles("/run/containerd/fifo/")
				assert.NilError(t, err)
				assert.Equal(t, oldNumFifos, newNumFifos)
			},
		}
	}

	testCase.Run(t)
}
