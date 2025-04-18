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

package network

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestNetworkInspect(t *testing.T) {
	testCase := nerdtest.Setup()

	const (
		testSubnet  = "10.24.24.0/24"
		testGateway = "10.24.24.1"
		testIPRange = "10.24.24.0/25"
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("network", "create", data.Identifier("basenet"))
		data.Labels().Set("basenet", data.Identifier("basenet"))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("network", "rm", data.Identifier("basenet"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "non existent network",
			Command:     test.Command("network", "inspect", "nonexistent"),
			// FIXME: where is this error even comin from?
			Expected: test.Expects(1, []error{errors.New("no network found matching")}, nil),
		},
		{
			Description: "invalid name network",
			Command:     test.Command("network", "inspect", "∞"),
			// FIXME: this is not even a valid identifier
			Expected: test.Expects(1, []error{errors.New("no network found matching")}, nil),
		},
		{
			Description: "none",
			Require:     nerdtest.NerdctlNeedsFixing("no issue opened"),
			Command:     test.Command("network", "inspect", "none"),
			Expected: test.Expects(0, nil, func(stdout string, info string, t *testing.T) {
				var dc []dockercompat.Network
				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(t, err, "Unable to unmarshal output\n"+info)
				assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
				assert.Equal(t, dc[0].Name, "none")
			}),
		},
		{
			Description: "host",
			Require:     nerdtest.NerdctlNeedsFixing("no issue opened"),
			Command:     test.Command("network", "inspect", "host"),
			Expected: test.Expects(0, nil, func(stdout string, info string, t *testing.T) {
				var dc []dockercompat.Network
				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(t, err, "Unable to unmarshal output\n"+info)
				assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
				assert.Equal(t, dc[0].Name, "host")
			}),
		},
		{
			Description: "bridge",
			Require:     require.Not(require.Windows),
			Command:     test.Command("network", "inspect", "bridge"),
			Expected: test.Expects(0, nil, func(stdout string, info string, t *testing.T) {
				var dc []dockercompat.Network
				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(t, err, "Unable to unmarshal output\n"+info)
				assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
				assert.Equal(t, dc[0].Name, "bridge")
			}),
		},
		{
			Description: "nat",
			Require:     require.Windows,
			Command:     test.Command("network", "inspect", "nat"),
			Expected: test.Expects(0, nil, func(stdout string, info string, t *testing.T) {
				var dc []dockercompat.Network
				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(t, err, "Unable to unmarshal output\n"+info)
				assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
				assert.Equal(t, dc[0].Name, "nat")
			}),
		},
		{
			Description: "custom",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("network", "create", "custom")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "remove", "custom")
			},
			Command: test.Command("network", "inspect", "custom"),
			Expected: test.Expects(0, nil, func(stdout string, info string, t *testing.T) {
				var dc []dockercompat.Network
				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(t, err, "Unable to unmarshal output\n"+info)
				assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
				assert.Equal(t, dc[0].Name, "custom")
			}),
		},
		{
			Description: "match exact id",
			// See notes below
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				id := strings.TrimSpace(helpers.Capture("network", "inspect", data.Labels().Get("basenet"), "--format", "{{ .Id }}"))
				return helpers.Command("network", "inspect", id)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						var dc []dockercompat.Network
						err := json.Unmarshal([]byte(stdout), &dc)
						assert.NilError(t, err, "Unable to unmarshal output\n"+info)
						assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
						assert.Equal(t, dc[0].Name, data.Labels().Get("basenet"))
					},
				}
			},
		},
		{
			Description: "match part of id",
			// FIXME: for windows, network inspect testnetworkinspect-basenet-468cf999 --format {{ .Id }} MAY fail here
			// This is bizarre, as it is working in the match exact id test - and there does not seem to be a particular reason for that
			Require: require.Not(require.Windows),
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				id := strings.TrimSpace(helpers.Capture("network", "inspect", data.Labels().Get("basenet"), "--format", "{{ .Id }}"))
				return helpers.Command("network", "inspect", id[0:25])
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						var dc []dockercompat.Network
						err := json.Unmarshal([]byte(stdout), &dc)
						assert.NilError(t, err, "Unable to unmarshal output\n"+info)
						assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
						assert.Equal(t, dc[0].Name, data.Labels().Get("basenet"))
					},
				}
			},
		},
		{
			Description: "using another net short id",
			// FIXME: for windows, network inspect testnetworkinspect-basenet-468cf999 --format {{ .Id }} MAY fail here
			// This is bizarre, as it is working in the match exact id test - and there does not seem to be a particular reason for that
			Require: require.Not(require.Windows),
			Setup: func(data test.Data, helpers test.Helpers) {
				id := strings.TrimSpace(helpers.Capture("network", "inspect", data.Labels().Get("basenet"), "--format", "{{ .Id }}"))
				helpers.Ensure("network", "create", id[0:12])
				data.Labels().Set("netname", id[0:12])
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "remove", data.Labels().Get("netname"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("network", "inspect", data.Labels().Get("netname"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						var dc []dockercompat.Network
						err := json.Unmarshal([]byte(stdout), &dc)
						assert.NilError(t, err, "Unable to unmarshal output\n"+info)
						assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
						assert.Equal(t, dc[0].Name, data.Labels().Get("netname"))
					},
				}
			},
		},
		{
			Description: "basic",
			// FIXME: IPAMConfig is not implemented on Windows yet
			Require: require.Not(require.Windows),
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("network", "create", "--label", "tag=testNetwork", "--subnet", testSubnet,
					"--gateway", testGateway, "--ip-range", testIPRange, data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "rm", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("network", "inspect", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, info string, t *testing.T) {
						var dc []dockercompat.Network

						err := json.Unmarshal([]byte(stdout), &dc)
						assert.NilError(t, err, "Unable to unmarshal output\n"+info)
						assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
						got := dc[0]

						assert.Equal(t, got.Name, data.Identifier(), info)
						assert.Equal(t, got.Labels["tag"], "testNetwork", info)
						assert.Equal(t, len(got.IPAM.Config), 1, info)
						assert.Equal(t, got.IPAM.Config[0].Subnet, testSubnet, info)
						assert.Equal(t, got.IPAM.Config[0].Gateway, testGateway, info)
						assert.Equal(t, got.IPAM.Config[0].IPRange, testIPRange, info)
					},
				}
			},
		},
		{
			Description: "with namespace",
			Require:     require.Not(nerdtest.Docker),
			Cleanup: func(data test.Data, helpers test.Helpers) {
				identifier := data.Identifier()
				helpers.Anyhow("network", "rm", identifier)
				helpers.Anyhow("namespace", "remove", identifier)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("network", "create", data.Identifier())
			},

			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, info string, t *testing.T) {
						cmd := helpers.Custom("nerdctl", "--namespace", data.Identifier())

						com := cmd.Clone()
						com.WithArgs("network", "inspect", data.Identifier())
						com.Run(&test.Expected{
							ExitCode: 1,
							Errors:   []error{errors.New("no network found")},
						})

						com = cmd.Clone()
						com.WithArgs("network", "remove", data.Identifier())
						com.Run(&test.Expected{
							ExitCode: 1,
							Errors:   []error{errors.New("no network found")},
						})

						com = cmd.Clone()
						com.WithArgs("network", "ls")
						com.Run(&test.Expected{
							Output: expect.DoesNotContain(data.Identifier()),
						})

						com = cmd.Clone()
						com.WithArgs("network", "prune", "-f")
						com.Run(&test.Expected{
							Output: expect.DoesNotContain(data.Identifier()),
						})
					},
				}
			},
		},
		{
			Description: "Verify that only active containers appear in the network inspect output",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("network", "create", data.Identifier("nginx-network-1"))
				helpers.Ensure("network", "create", data.Identifier("nginx-network-2"))
				helpers.Ensure("create", "--name", data.Identifier("nginx-container-1"), "--network", data.Identifier("nginx-network-1"), testutil.NginxAlpineImage)
				helpers.Ensure("create", "--name", data.Identifier("nginx-container-2"), "--network", data.Identifier("nginx-network-1"), testutil.NginxAlpineImage)
				helpers.Ensure("create", "--name", data.Identifier("nginx-container-on-diff-network"), "--network", data.Identifier("nginx-network-2"), testutil.NginxAlpineImage)
				helpers.Ensure("start", data.Identifier("nginx-container-1"), data.Identifier("nginx-container-on-diff-network"))
				data.Labels().Set("nginx-container-1-id", strings.Trim(helpers.Capture("inspect", data.Identifier("nginx-container-1"), "--format", "{{.Id}}"), "\n"))
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier("nginx-container-1"))
				helpers.Anyhow("rm", "-f", data.Identifier("nginx-container-2"))
				helpers.Anyhow("rm", "-f", data.Identifier("nginx-container-on-diff-network"))
				helpers.Anyhow("network", "remove", data.Identifier("nginx-network-1"))
				helpers.Anyhow("network", "remove", data.Identifier("nginx-network-2"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("network", "inspect", data.Identifier("nginx-network-1"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						var dc []dockercompat.Network
						err := json.Unmarshal([]byte(stdout), &dc)
						assert.NilError(t, err, "Unable to unmarshal output\n"+info)
						assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
						assert.Equal(t, dc[0].Name, data.Identifier("nginx-network-1"))
						// Assert only the "running" containers on the same network are returned.
						assert.Equal(t, 1, len(dc[0].Containers), "Expected a single container as per configuration, but got multiple.")
						assert.Equal(t, data.Identifier("nginx-container-1"), dc[0].Containers[data.Labels().Get("nginx-container-1-id")].Name)
					},
				}
			},
		},
	}

	testCase.Run(t)
}
