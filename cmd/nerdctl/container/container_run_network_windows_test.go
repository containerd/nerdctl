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
	"regexp"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/defaults"
	"github.com/containerd/nerdctl/v2/pkg/netutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

// TestRunInternetConnectivity tests Internet connectivity by pinging github.com.
func TestRunInternetConnectivity(t *testing.T) {
	testCase := nerdtest.Setup()
	// TODO(aznashwan): smarter way to ensure internet connectivity is working.
	// ping doesn't seem to work on GitHub Actions ("Request timed out.")
	testCase.Command = test.Command("run", "--rm", "--net", "nat", testutil.CommonImage, "curl.exe -sSL https://github.com")
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("<!DOCTYPE html>"))
	testCase.Run(t)
}

func TestRunPort(t *testing.T) {
	// NOTE: currently no isolation between the loopback and host namespaces on Windows.
	baseTestRunPort(t, testutil.NginxAlpineImage, testutil.NginxAlpineIndexHTMLSnippet, false)
}

// Checks whether an HNS endpoint with a name matching exists.
func listHnsEndpointsRegex(hnsEndpointNameRegex string) ([]hcsshim.HNSEndpoint, error) {
	r, err := regexp.Compile(hnsEndpointNameRegex)
	if err != nil {
		return nil, err
	}
	hnsEndpoints, err := hcsshim.HNSListEndpointRequest()
	if err != nil {
		return nil, fmt.Errorf("failed to list HNS endpoints for request: %w", err)
	}

	res := []hcsshim.HNSEndpoint{}
	for _, endp := range hnsEndpoints {
		if r.Match([]byte(endp.Name)) {
			res = append(res, endp)
		}
	}
	return res, nil
}

// Asserts whether the container with the provided has any HNS endpoints with the expected
// naming format (`${container_id}_${network_name}`) for all of the provided network names.
// The container ID can be a regex.
func assertHnsEndpointsExistence(helpers test.Helpers, shouldExist bool, containerIDRegex string, networkNames ...string) {
	helpers.T().Helper()
	for _, netName := range networkNames {
		endpointName := fmt.Sprintf("%s_%s", containerIDRegex, netName)
		matchingEndpoints, err := listHnsEndpointsRegex(endpointName)
		assert.NilError(helpers.T(), err)
		if shouldExist {
			assert.Equal(helpers.T(), len(matchingEndpoints), 1)
			assert.Equal(helpers.T(), matchingEndpoints[0].Name, endpointName)
		} else {
			assert.Equal(helpers.T(), len(matchingEndpoints), 0)
		}
	}
}

// Tests whether HNS endpoints are properly created and managed throughout the lifecycle of a container.
func TestHnsEndpointsExistDuringContainerLifecycle(t *testing.T) {
	testCase := nerdtest.Setup()
	// This test inspects host-wide HNS endpoint state on the shared default network,
	// which is not safe to run in parallel with other tests touching the same network.
	testCase.NoParallel = true

	var netName string
	var containerID string

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		testNet, err := getTestingNetwork()
		assert.NilError(helpers.T(), err)
		netName = testNet.Name

		// NOTE: the BusyBox image used in Windows testing's `sleep` binary
		// does not support the `infinity` argument.
		createOut := helpers.Capture(
			"create",
			"--name", data.Identifier(),
			"--net", testNet.Name,
			testutil.CommonImage,
			"bash", "-c",
			"tail", "-f",
		)
		containerID = strings.TrimSpace(createOut)
		helpers.T().Log(fmt.Sprintf("HNS endpoint lifecycle test container ID: %q", containerID))

		// HNS endpoints should be allocated on container creation.
		assertHnsEndpointsExistence(helpers, true, containerID, netName)

		// Starting and stopping the container should NOT affect/change the endpoints.
		helpers.Ensure("start", containerID)
		assertHnsEndpointsExistence(helpers, true, containerID, netName)

		helpers.Ensure("stop", containerID)
		assertHnsEndpointsExistence(helpers, true, containerID, netName)
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// Removing the container should remove the HNS endpoints.
		return helpers.Command("rm", containerID)
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				assertHnsEndpointsExistence(helpers, false, containerID, netName)
			},
		}
	}
	testCase.Run(t)
}

// Returns a network to be used for testing.
// Note: currently hardcoded to return the default network, as `network create`
// does not work on Windows.
func getTestingNetwork() (*netutil.NetworkConfig, error) {
	// NOTE: cannot currently `nerdctl network create` on Windows so we use a pre-existing network:
	cniEnv, err := netutil.NewCNIEnv(defaults.CNIPath(), defaults.CNINetConfPath())
	if err != nil {
		return nil, err
	}

	return cniEnv.GetDefaultNetworkConfig()
}

// Tests whether HNS endpoints are properly removed when running `run --rm`.
func TestHnsEndpointsRemovedAfterAttachedRun(t *testing.T) {
	testCase := nerdtest.Setup()
	// This test counts host-wide HNS endpoints on the shared default network before and after
	// the run; concurrent tests creating/removing containers on the same network would corrupt
	// the count. Container cleanup is handled by `--rm`, so no Cleanup callback is needed.
	testCase.NoParallel = true

	var netName string
	var originalEndpointsCount int

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		testNet, err := getTestingNetwork()
		assert.NilError(helpers.T(), err)
		netName = testNet.Name

		// NOTE: because we cannot set/obtain the ID of the container to check for the exact HNS
		// endpoint name, we record the number of HNS endpoints on the testing network and
		// ensure it remains constant until after the test.
		existingEndpoints, err := listHnsEndpointsRegex(fmt.Sprintf(".*_%s", testNet.Name))
		assert.NilError(helpers.T(), err)
		originalEndpointsCount = len(existingEndpoints)
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command(
			"run",
			"--name", data.Identifier(),
			"--rm",
			"--net", netName,
			testutil.CommonImage,
			"ipconfig", "/all",
		)
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				existingEndpoints, err := listHnsEndpointsRegex(fmt.Sprintf(".*_%s", netName))
				assert.NilError(t, err)
				assert.Equal(t, originalEndpointsCount, len(existingEndpoints), "the number of HNS endpoints should equal pre-test amount")
			},
		}
	}
	testCase.Run(t)
}
