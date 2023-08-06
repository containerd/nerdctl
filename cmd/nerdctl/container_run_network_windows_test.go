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
	"regexp"
	"strings"
	"testing"

	"github.com/Microsoft/hcsshim"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

// TestRunInternetConnectivity tests Internet connectivity by pinging github.com.
func TestRunInternetConnectivity(t *testing.T) {
	base := testutil.NewBase(t)

	type testCase struct {
		args []string
	}
	testCases := []testCase{
		{
			args: []string{"--net", "nat"},
		},
	}
	for _, tc := range testCases {
		tc := tc // IMPORTANT
		name := "default"
		if len(tc.args) > 0 {
			name = strings.Join(tc.args, "_")
		}
		t.Run(name, func(t *testing.T) {
			args := []string{"run", "--rm"}
			args = append(args, tc.args...)
			// TODO(aznashwan): smarter way to ensure internet connectivity is working.
			// ping doesn't seem to work on GitHub Actions ("Request timed out.")
			args = append(args, testutil.CommonImage, "curl.exe -sSL https://github.com")
			cmd := base.Cmd(args...)
			cmd.AssertOutContains("<!DOCTYPE html>")
		})
	}
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
		return nil, fmt.Errorf("failed to list HNS endpoints for request: %s", err)
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
func assertHnsEndpointsExistence(t *testing.T, shouldExist bool, containerIDRegex string, networkNames ...string) {
	for _, netName := range networkNames {
		endpointName := fmt.Sprintf("%s_%s", containerIDRegex, netName)

		testName := fmt.Sprintf("hns_endpoint_%s_shouldExist_%t", endpointName, shouldExist)
		t.Run(testName, func(t *testing.T) {
			matchingEndpoints, err := listHnsEndpointsRegex(endpointName)
			assert.NilError(t, err)
			if shouldExist {
				assert.Equal(t, len(matchingEndpoints), 1)
				assert.Equal(t, matchingEndpoints[0].Name, endpointName)
			} else {
				assert.Equal(t, len(matchingEndpoints), 0)
			}
		})
	}
}

// Tests whether HNS endpoints are properly created and managed throughout the lifecycle of a container.
func TestHnsEndpointsExistDuringContainerLifecycle(t *testing.T) {
	base := testutil.NewBase(t)

	testNet, err := getTestingNetwork()
	assert.NilError(t, err)

	tID := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", tID).Run()
	cmd := base.Cmd(
		"create",
		"--name", tID,
		"--net", testNet.Name,
		testutil.CommonImage,
		"bash", "-c",
		// NOTE: the BusyBox image used in Windows testing's `sleep` binary
		// does not support the `infinity` argument.
		"tail", "-f",
	)
	t.Logf("Creating HNS lifecycle test container with command: %q", strings.Join(cmd.Command, " "))
	containerId := strings.TrimSpace(cmd.Run().Stdout())
	t.Logf("HNS endpoint lifecycle test container ID: %q", containerId)

	// HNS endpoints should be allocated on container creation.
	assertHnsEndpointsExistence(t, true, containerId, testNet.Name)

	// Starting and stopping the container should NOT affect/change the endpoints.
	base.Cmd("start", containerId).AssertOK()
	assertHnsEndpointsExistence(t, true, containerId, testNet.Name)

	base.Cmd("stop", containerId).AssertOK()
	assertHnsEndpointsExistence(t, true, containerId, testNet.Name)

	// Removing the container should remove the HNS endpoints.
	base.Cmd("rm", containerId).AssertOK()
	assertHnsEndpointsExistence(t, false, containerId, testNet.Name)
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
	base := testutil.NewBase(t)

	testNet, err := getTestingNetwork()
	assert.NilError(t, err)

	// NOTE: because we cannot set/obtain the ID of the container to check for the exact HNS
	// endpoint name, we record the number of HNS endpoints on the testing network and
	// ensure it remains constant until after the test.
	existingEndpoints, err := listHnsEndpointsRegex(fmt.Sprintf(".*_%s", testNet.Name))
	assert.NilError(t, err)
	originalEndpointsCount := len(existingEndpoints)

	tID := testutil.Identifier(t)
	base.Cmd(
		"run",
		"--name",
		tID,
		"--rm",
		"--net", testNet.Name,
		testutil.CommonImage,
		"ipconfig", "/all",
	).AssertOK()

	existingEndpoints, err = listHnsEndpointsRegex(fmt.Sprintf(".*_%s", testNet.Name))
	assert.NilError(t, err)
	assert.Equal(t, originalEndpointsCount, len(existingEndpoints), "the number of HNS endpoints should equal pre-test amount")
}
