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

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestNetworkInspect(t *testing.T) {
	testCase := nerdtest.Setup()

	const (
		testSubnet  = "10.24.24.0/24"
		testGateway = "10.24.24.1"
		testIPRange = "10.24.24.0/25"
	)

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
			Require:     test.Not(test.Windows),
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
			Require:     test.Not(test.Windows),
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				id := strings.TrimSpace(helpers.Capture("network", "inspect", "bridge", "--format", "{{ .Id }}"))
				return helpers.Command("network", "inspect", id)
			},
			Expected: test.Expects(0, nil, func(stdout string, info string, t *testing.T) {
				var dc []dockercompat.Network
				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(t, err, "Unable to unmarshal output\n"+info)
				assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
				assert.Equal(t, dc[0].Name, "bridge")
			}),
		},
		{
			Description: "match part of id",
			Require:     test.Not(test.Windows),
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				id := strings.TrimSpace(helpers.Capture("network", "inspect", "bridge", "--format", "{{ .Id }}"))
				return helpers.Command("network", "inspect", id[0:25])
			},
			Expected: test.Expects(0, nil, func(stdout string, info string, t *testing.T) {
				var dc []dockercompat.Network
				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(t, err, "Unable to unmarshal output\n"+info)
				assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
				assert.Equal(t, dc[0].Name, "bridge")
			}),
		},
		{
			Description: "using another net short id",
			Require:     test.Not(test.Windows),
			Setup: func(data test.Data, helpers test.Helpers) {
				id := strings.TrimSpace(helpers.Capture("network", "inspect", "bridge", "--format", "{{ .Id }}"))
				helpers.Ensure("network", "create", id[0:12])
				data.Set("netname", id[0:12])
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				id := strings.TrimSpace(helpers.Capture("network", "inspect", "bridge", "--format", "{{ .Id }}"))
				helpers.Anyhow("network", "remove", id[0:12])
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("network", "inspect", data.Get("netname"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						var dc []dockercompat.Network
						err := json.Unmarshal([]byte(stdout), &dc)
						assert.NilError(t, err, "Unable to unmarshal output\n"+info)
						assert.Equal(t, 1, len(dc), "Unexpectedly got multiple results\n"+info)
						assert.Equal(t, dc[0].Name, data.Get("netname"))
					},
				}
			},
		},
		{
			Description: "basic",
			// IPAMConfig is not implemented on Windows yet
			Require: test.Not(test.Windows),
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
			Require:     test.Not(nerdtest.Docker),
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("network", "rm", data.Identifier())
				helpers.Anyhow("namespace", "remove", data.Identifier())
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
							Output: test.DoesNotContain(data.Identifier()),
						})

						com = cmd.Clone()
						com.WithArgs("network", "prune", "-f")
						com.Run(&test.Expected{
							Output: test.DoesNotContain(data.Identifier()),
						})
					},
				}
			},
		},
	}

	testCase.Run(t)
}
