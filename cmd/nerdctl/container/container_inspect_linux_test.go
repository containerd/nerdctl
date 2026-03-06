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
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/docker/go-connections/nat"
	"gotest.tools/v3/assert"

	"github.com/containerd/continuity/testutil/loopback"
	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/portlock"
)

func TestContainerInspectContainsPortConfig(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		_, err := portlock.Acquire(8080)
		if err != nil {
			t.Logf("Failed to acquire port: %v", err)
			t.FailNow()
		}
		helpers.Ensure("run", "-d", "--name", data.Identifier(), "-p", "8080:80", testutil.NginxAlpineImage)
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
		portlock.Release(8080)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, tt tig.T) {
		var dc []dockercompat.Container

		err := json.Unmarshal([]byte(stdout), &dc)
		assert.NilError(tt, err)
		assert.Equal(tt, 1, len(dc))

		inspect80TCP := (*dc[0].NetworkSettings.Ports)["80/tcp"]
		expected := nat.PortBinding{
			HostIP:   "0.0.0.0",
			HostPort: "8080",
		}
		assert.Equal(tt, expected, inspect80TCP[0])
	})

	testCase.Run(t)
}

func TestContainerInspectContainsMounts(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		testContainer := data.Identifier()
		testVolume := data.Identifier()

		helpers.Ensure("volume", "create", "--label", "tag=testVolume", testVolume)
		inspectVolume := nerdtest.InspectVolume(helpers, testVolume)
		namedVolumeSource := inspectVolume.Mountpoint

		helpers.Ensure("run", "-d", "--privileged",
			"--name", testContainer,
			"--network", "none",
			"-v", "/anony-vol",
			"--tmpfs", "/app1:size=64m",
			"--mount", "type=bind,src=/tmp,dst=/app2,ro",
			"--mount", fmt.Sprintf("type=volume,src=%s,dst=/app3,readonly=false", testVolume),
			testutil.NginxAlpineImage)
		nerdtest.EnsureContainerStarted(helpers, testContainer)

		data.Labels().Set("namedVolumeSource", namedVolumeSource)
		data.Labels().Set("testVolume", testVolume)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
		helpers.Anyhow("volume", "rm", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, tt tig.T) {
				var dc []dockercompat.Container

				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(tt, err)
				assert.Equal(tt, 1, len(dc))

				inspect := dc[0]
				// convert array to map to get by key of Destination
				actual := make(map[string]dockercompat.MountPoint)
				for i := range inspect.Mounts {
					actual[inspect.Mounts[i].Destination] = inspect.Mounts[i]
				}

				t.Logf("actual in TestContainerInspectContainsMounts: %+v", actual)
				const localDriver = "local"

				expected := []struct {
					dest       string
					mountPoint dockercompat.MountPoint
				}{
					// anonymous volume
					{
						dest: "/anony-vol",
						mountPoint: dockercompat.MountPoint{
							Type:        "volume",
							Name:        "",
							Source:      "", // source of anonymous volume is a generated path, so here will not check it.
							Destination: "/anony-vol",
							Driver:      localDriver,
							RW:          true,
						},
					},

					// bind
					{
						dest: "/app2",
						mountPoint: dockercompat.MountPoint{
							Type:        "bind",
							Name:        "",
							Source:      "/tmp",
							Destination: "/app2",
							Driver:      "",
							RW:          false,
						},
					},

					// named volume
					{
						dest: "/app3",
						mountPoint: dockercompat.MountPoint{
							Type:        "volume",
							Name:        data.Labels().Get("testVolume"),
							Source:      data.Labels().Get("namedVolumeSource"),
							Destination: "/app3",
							Driver:      localDriver,
							RW:          true,
						},
					},
				}

				for i := range expected {
					mountCase := expected[i]
					t.Logf("test volume[dest=%q]", mountCase.dest)

					mountPoint, ok := actual[mountCase.dest]
					assert.Assert(tt, ok)

					assert.Equal(tt, mountCase.mountPoint.Type, mountPoint.Type)
					assert.Equal(tt, mountCase.mountPoint.Driver, mountPoint.Driver)
					assert.Equal(tt, mountCase.mountPoint.RW, mountPoint.RW)
					assert.Equal(tt, mountCase.mountPoint.Destination, mountPoint.Destination)

					if mountCase.mountPoint.Source != "" {
						assert.Equal(tt, mountCase.mountPoint.Source, mountPoint.Source)
					}
					if mountCase.mountPoint.Name != "" {
						assert.Equal(tt, mountCase.mountPoint.Name, mountPoint.Name)
					}
				}
			},
		}
	}

	testCase.Run(t)
}

func TestContainerInspectContainsLabel(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier(), "--label", "foo=foo", "--label", "bar=bar", testutil.NginxAlpineImage)
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, tt tig.T) {
		var dc []dockercompat.Container

		err := json.Unmarshal([]byte(stdout), &dc)
		assert.NilError(tt, err)
		assert.Equal(tt, 1, len(dc))

		inspect := dc[0]
		lbs := inspect.Config.Labels

		assert.Equal(tt, "foo", lbs["foo"])
		assert.Equal(tt, "bar", lbs["bar"])
	})

	testCase.Run(t)
}

func TestContainerInspectContainsInternalLabel(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.Not(nerdtest.Docker)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier(), "--mount", "type=bind,src=/tmp,dst=/app,readonly=false,bind-propagation=rprivate", testutil.NginxAlpineImage)
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, tt tig.T) {
		var dc []dockercompat.Container

		err := json.Unmarshal([]byte(stdout), &dc)
		assert.NilError(tt, err)
		assert.Equal(tt, 1, len(dc))

		inspect := dc[0]
		lbs := inspect.Config.Labels

		// TODO: add more internal labels testcases
		labelMount := lbs[labels.Mounts]
		expectedLabelMount := "[{\"Type\":\"bind\",\"Source\":\"/tmp\",\"Destination\":\"/app\",\"Mode\":\"rprivate,rbind\",\"RW\":true,\"Propagation\":\"rprivate\"}]"
		assert.Equal(tt, expectedLabelMount, labelMount)
	})

	testCase.Run(t)
}

func TestContainerInspectConfigImage(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Description: "Container inspect contains Config.Image field",
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.AlpineImage, "sleep", nerdtest.Infinity)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rm", "-f", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("inspect", data.Identifier())
		},
		Expected: test.Expects(0, nil, func(stdout string, tt tig.T) {
			var containers []dockercompat.Container
			err := json.Unmarshal([]byte(stdout), &containers)
			assert.NilError(tt, err, "Unable to unmarshal output\n")
			assert.Equal(tt, 1, len(containers), "Expected exactly one container in inspect output")

			container := containers[0]
			assert.Assert(tt, container.Config != nil, "container Config should not be nil")
			assert.Assert(tt, container.Config.Image != "", "Config.Image should not be empty")
		}),
	}

	testCase.Run(t)
}

func TestContainerInspectState(t *testing.T) {
	testCase := nerdtest.Setup()

	// nerdctl: run error produces a nil Task, so the Status is empty because Status comes from Task.
	// docker : run error gives => `Status=created` as  in docker there is no a separation between container and Task.
	testCase.SubTests = []*test.Case{
		{
			Description: "docker inspect State with error",
			Setup: func(data test.Data, helpers test.Helpers) {
				testContainer := fmt.Sprintf("%s-fail", data.Identifier())
				helpers.Fail("run", "--name", testContainer, testutil.AlpineImage, "aa")
				data.Labels().Set("testContainer", testContainer)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Labels().Get("testContainer"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("inspect", data.Labels().Get("testContainer"))
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, tt tig.T) {
				var dc []dockercompat.Container

				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(tt, err)
				assert.Equal(tt, 1, len(dc))

				inspect := dc[0]
				expectedErrStatus := ""
				if nerdtest.IsDocker() {
					expectedErrStatus = "created"
				}
				assert.Assert(tt, strings.Contains(inspect.State.Error, "executable file not found in $PATH"), fmt.Sprintf("expected: %s, actual: %s", "executable file not found in $PATH", inspect.State.Error))
				assert.Equal(tt, expectedErrStatus, inspect.State.Status)
			}),
		},
		{
			Description: "docker inspect State without error",
			Setup: func(data test.Data, helpers test.Helpers) {
				testContainer := fmt.Sprintf("%s-success", data.Identifier())
				helpers.Ensure("run", "--name", testContainer, testutil.AlpineImage, "ls")
				data.Labels().Set("testContainer", testContainer)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Labels().Get("testContainer"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("inspect", data.Labels().Get("testContainer"))
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, tt tig.T) {
				var dc []dockercompat.Container

				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(tt, err)
				assert.Equal(tt, 1, len(dc))

				inspect := dc[0]
				assert.Assert(tt, strings.Contains(inspect.State.Error, ""), fmt.Sprintf("expected: %s, actual: %s", "", inspect.State.Error))
				assert.Equal(tt, "exited", inspect.State.Status)
			}),
		},
	}

	testCase.Run(t)
}

func TestContainerInspectHostConfig(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.Not(
		// skip only if it's rootless AND cgroup v1
		require.All(
			nerdtest.Rootless,
			require.Not(nerdtest.CGroupV2),
		),
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier(),
			"--cpuset-cpus", "0-1",
			"--cpuset-mems", "0",
			"--cpu-shares", "1024",
			"--cpu-quota", "100000",
			"--group-add", "1000",
			"--group-add", "2000",
			"--add-host", "host1:10.0.0.1",
			"--add-host", "host2:10.0.0.2",
			"--ipc", "host",
			"--memory", "512m",
			"--read-only",
			"--shm-size", "256m",
			"--uts", "host",
			"--runtime", "io.containerd.runc.v2",
			testutil.AlpineImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, tt tig.T) {
		var dc []dockercompat.Container

		err := json.Unmarshal([]byte(stdout), &dc)
		assert.NilError(tt, err)
		assert.Equal(tt, 1, len(dc))

		inspect := dc[0]

		assert.Equal(tt, "0-1", inspect.HostConfig.CPUSetCPUs)
		assert.Equal(tt, "0", inspect.HostConfig.CPUSetMems)
		assert.Equal(tt, uint64(1024), inspect.HostConfig.CPUShares)
		assert.Equal(tt, int64(100000), inspect.HostConfig.CPUQuota)
		assert.Assert(tt, slices.Contains(inspect.HostConfig.GroupAdd, "1000"), "Expected '1000' to be in GroupAdd")
		assert.Assert(tt, slices.Contains(inspect.HostConfig.GroupAdd, "2000"), "Expected '2000' to be in GroupAdd")
		expectedExtraHosts := []string{"host1:10.0.0.1", "host2:10.0.0.2"}
		assert.DeepEqual(tt, expectedExtraHosts, inspect.HostConfig.ExtraHosts)
		assert.Equal(tt, "host", inspect.HostConfig.IpcMode)
		assert.Equal(tt, int64(536870912), inspect.HostConfig.Memory)
		assert.Equal(tt, int64(1073741824), inspect.HostConfig.MemorySwap)
		assert.Equal(tt, true, inspect.HostConfig.ReadonlyRootfs)
		assert.Equal(tt, "host", inspect.HostConfig.UTSMode)
		assert.Equal(tt, int64(268435456), inspect.HostConfig.ShmSize)
	})

	testCase.Run(t)
}

func TestContainerInspectHostConfigDefaults(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		var hc hostConfigValues

		// Hostconfig default values differ with Docker.
		// This is because we directly retrieve the configured values instead of using preset defaults.
		if nerdtest.IsDocker() {
			hc.Driver = ""
			hc.GroupAddSize = 0
			hc.ShmSize = int64(67108864) // Docker default 64M
			hc.Runtime = "runc"
		} else {
			hc.GroupAddSize = 10
			hc.Driver = "json-file"
			hc.ShmSize = int64(0)
			hc.Runtime = "io.containerd.runc.v2"
		}

		helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.AlpineImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())

		jsonHC, err := json.Marshal(hc)
		assert.NilError(t, err)
		data.Labels().Set("jsonHC", string(jsonHC))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, tt tig.T) {
				var hc hostConfigValues
				err := json.Unmarshal([]byte(data.Labels().Get("jsonHC")), &hc)
				assert.NilError(tt, err)

				var dc []dockercompat.Container

				err = json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(tt, err)
				assert.Equal(tt, 1, len(dc))

				inspect := dc[0]
				t.Logf("HostConfig in TestContainerInspectHostConfigDefaults: %+v", inspect.HostConfig)
				assert.Equal(tt, "", inspect.HostConfig.CPUSetCPUs)
				assert.Equal(tt, "", inspect.HostConfig.CPUSetMems)
				assert.Equal(tt, uint16(0), inspect.HostConfig.BlkioWeight)
				assert.Equal(tt, 0, len(inspect.HostConfig.BlkioWeightDevice))
				assert.Equal(tt, 0, len(inspect.HostConfig.BlkioDeviceReadBps))
				assert.Equal(tt, 0, len(inspect.HostConfig.BlkioDeviceReadIOps))
				assert.Equal(tt, 0, len(inspect.HostConfig.BlkioDeviceWriteBps))
				assert.Equal(tt, 0, len(inspect.HostConfig.BlkioDeviceWriteIOps))
				assert.Equal(tt, uint64(0), inspect.HostConfig.CPUShares)
				assert.Equal(tt, int64(0), inspect.HostConfig.CPUQuota)
				assert.Equal(tt, hc.GroupAddSize, len(inspect.HostConfig.GroupAdd))
				assert.Equal(tt, 0, len(inspect.HostConfig.ExtraHosts))
				assert.Equal(tt, "private", inspect.HostConfig.IpcMode)
				assert.Equal(tt, hc.Driver, inspect.HostConfig.LogConfig.Driver)
				assert.Equal(tt, int64(0), inspect.HostConfig.Memory)
				assert.Equal(tt, int64(0), inspect.HostConfig.MemorySwap)
				assert.Equal(tt, bool(false), inspect.HostConfig.OomKillDisable)
				assert.Equal(tt, bool(false), inspect.HostConfig.ReadonlyRootfs)
				assert.Equal(tt, "", inspect.HostConfig.UTSMode)
				assert.Equal(tt, hc.ShmSize, inspect.HostConfig.ShmSize)
				assert.Equal(tt, hc.Runtime, inspect.HostConfig.Runtime)
				assert.Equal(tt, 0, len(inspect.HostConfig.Devices))

				// Sysctls can be empty or contain "net.ipv4.ip_unprivileged_port_start" depending on the environment.
				got := len(inspect.HostConfig.Sysctls)
				if got != 0 && got != 1 {
					t.Fatalf("unexpected number of Sysctls entries: %d (want 0 or 1)", got)
				}
			},
		}
	}

	testCase.Run(t)
}

func TestContainerInspectHostConfigDNS(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier(),
			"--dns", "8.8.8.8",
			"--dns", "1.1.1.1",
			"--dns-search", "example.com",
			"--dns-search", "test.local",
			"--dns-option", "ndots:5",
			"--dns-option", "timeout:3",
			testutil.AlpineImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, tt tig.T) {
		var dc []dockercompat.Container
		err := json.Unmarshal([]byte(stdout), &dc)
		assert.NilError(tt, err)
		assert.Equal(tt, 1, len(dc))

		inspect := dc[0]
		// Check DNS servers
		expectedDNSServers := []string{"8.8.8.8", "1.1.1.1"}
		assert.DeepEqual(tt, expectedDNSServers, inspect.HostConfig.DNS)

		// Check DNS search domains
		expectedDNSSearch := []string{"example.com", "test.local"}
		assert.DeepEqual(tt, expectedDNSSearch, inspect.HostConfig.DNSSearch)

		// Check DNS options
		expectedDNSOptions := []string{"ndots:5", "timeout:3"}
		assert.DeepEqual(tt, expectedDNSOptions, inspect.HostConfig.DNSOptions)
	})

	testCase.Run(t)
}

func TestContainerInspectHostConfigDNSDefaults(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.AlpineImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, tt tig.T) {
		var dc []dockercompat.Container

		err := json.Unmarshal([]byte(stdout), &dc)
		assert.NilError(tt, err)
		assert.Equal(tt, 1, len(dc))

		inspect := dc[0]

		// Check that DNS settings are empty by default
		assert.Equal(tt, 0, len(inspect.HostConfig.DNS))
		assert.Equal(tt, 0, len(inspect.HostConfig.DNSSearch))
		assert.Equal(tt, 0, len(inspect.HostConfig.DNSOptions))
	})

	testCase.Run(t)
}

func TestContainerInspectHostConfigPID(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		testContainer1 := data.Identifier() + "-container1"
		testContainer2 := data.Identifier() + "-container2"

		// Run the first container
		helpers.Ensure("run", "-d", "--name", testContainer1, testutil.AlpineImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, testContainer1)

		containerID1 := strings.TrimSpace(helpers.Capture("inspect", "-f", "{{.Id}}", testContainer1))

		var pidMode string
		if nerdtest.IsDocker() {
			pidMode = "container:" + containerID1
		} else {
			pidMode = containerID1
		}

		helpers.Ensure("run", "-d", "--name", testContainer2, "--pid", fmt.Sprintf("container:%s", testContainer1), testutil.AlpineImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, testContainer2)

		data.Labels().Set("pidMode", pidMode)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier()+"-container1")
		helpers.Anyhow("rm", "-f", data.Identifier()+"-container2")
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", data.Identifier()+"-container2")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, tt tig.T) {
				var dc []dockercompat.Container

				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(tt, err)
				assert.Equal(tt, 1, len(dc))

				inspect := dc[0]
				assert.Equal(tt, data.Labels().Get("pidMode"), inspect.HostConfig.PidMode)
			},
		}
	}

	testCase.Run(t)

}

func TestContainerInspectHostConfigPIDDefaults(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.AlpineImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, tt tig.T) {
		var dc []dockercompat.Container

		err := json.Unmarshal([]byte(stdout), &dc)
		assert.NilError(tt, err)
		assert.Equal(tt, 1, len(dc))

		inspect := dc[0]

		assert.Equal(tt, "", inspect.HostConfig.PidMode)
	})

	testCase.Run(t)
}

func TestContainerInspectDevices(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.CgroupsAccessible

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// Create a temporary directory
		dir := data.Temp().Dir(t.TempDir(), "device-dir")

		if nerdtest.IsDocker() {
			dir = "/dev/zero"
		}

		helpers.Ensure("run", "-d", "--name", data.Identifier(), "--device", dir+":/dev/xvda", testutil.AlpineImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())

		data.Labels().Set("dir", dir)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, tt tig.T) {
				var dc []dockercompat.Container

				err := json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(tt, err)
				assert.Equal(tt, 1, len(dc))

				inspect := dc[0]
				expectedDevices := []dockercompat.DeviceMapping{
					{
						PathOnHost:        data.Labels().Get("dir"),
						PathInContainer:   "/dev/xvda",
						CgroupPermissions: "rwm",
					},
				}
				assert.DeepEqual(tt, expectedDevices, inspect.HostConfig.Devices)
			},
		}
	}

	testCase.Run(t)
}

func TestContainerInspectBlkioSettings(t *testing.T) {
	var lo *loopback.Loopback

	testCase := nerdtest.Setup()

	// Some of the blkio settings are not supported in cgroup v1.
	// So skip this test if running on cgroup v1
	testCase.Require = require.All(
		require.Not(nerdtest.Docker),
		require.Not(nerdtest.Rootless),
		require.Not(nerdtest.CGroup),
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// See https://github.com/containerd/nerdctl/issues/4185
		// It is unclear if this is truly a kernel version problem, a runc issue, or a distro (EL9) issue.
		// For now, disable the test unless on a recent kernel.
		testutil.RequireKernelVersion(t, ">= 6.0.0-0")

		var err error
		lo, err = loopback.New(4096)
		if err != nil {
			err = fmt.Errorf("cannot find a loop device: %w", err)
			t.Fatal(err)
		}

		const (
			weight    = 500
			readBps   = 1048576
			readIops  = 1000
			writeBps  = 2097152
			writeIops = 2000
		)

		helpers.Ensure("run", "-d", "--name", data.Identifier(),
			"--blkio-weight", fmt.Sprintf("%d", weight),
			"--blkio-weight-device", fmt.Sprintf("%s:%d", lo.Device, weight),
			"--device-read-bps", fmt.Sprintf("%s:%d", lo.Device, readBps),
			"--device-read-iops", fmt.Sprintf("%s:%d", lo.Device, readIops),
			"--device-write-bps", fmt.Sprintf("%s:%d", lo.Device, writeBps),
			"--device-write-iops", fmt.Sprintf("%s:%d", lo.Device, writeIops),
			testutil.AlpineImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())

		data.Labels().Set("weight", strconv.Itoa(weight))
		data.Labels().Set("readBps", strconv.Itoa(readBps))
		data.Labels().Set("readIops", strconv.Itoa(readIops))
		data.Labels().Set("writeBps", strconv.Itoa(writeBps))
		data.Labels().Set("writeIops", strconv.Itoa(writeIops))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
		if lo != nil {
			lo.Close()
		}
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, tt tig.T) {
				weight, err := strconv.Atoi(data.Labels().Get("weight"))
				assert.NilError(tt, err)
				readBps, err := strconv.Atoi(data.Labels().Get("readBps"))
				assert.NilError(tt, err)
				writeBps, err := strconv.Atoi(data.Labels().Get("writeBps"))
				assert.NilError(tt, err)
				readIops, err := strconv.Atoi(data.Labels().Get("readIops"))
				assert.NilError(tt, err)
				writeIops, err := strconv.Atoi(data.Labels().Get("writeIops"))
				assert.NilError(tt, err)

				var dc []dockercompat.Container

				err = json.Unmarshal([]byte(stdout), &dc)
				assert.NilError(tt, err)
				assert.Equal(tt, 1, len(dc))

				inspect := dc[0]

				assert.Equal(tt, uint16(weight), inspect.HostConfig.BlkioWeight)
				assert.Equal(tt, 1, len(inspect.HostConfig.BlkioWeightDevice))
				assert.Equal(tt, lo.Device, inspect.HostConfig.BlkioWeightDevice[0].Path)
				assert.Equal(tt, uint16(weight), inspect.HostConfig.BlkioWeightDevice[0].Weight)
				assert.Equal(tt, 1, len(inspect.HostConfig.BlkioDeviceReadBps))
				assert.Equal(tt, uint64(readBps), inspect.HostConfig.BlkioDeviceReadBps[0].Rate)
				assert.Equal(tt, 1, len(inspect.HostConfig.BlkioDeviceWriteBps))
				assert.Equal(tt, uint64(writeBps), inspect.HostConfig.BlkioDeviceWriteBps[0].Rate)
				assert.Equal(tt, 1, len(inspect.HostConfig.BlkioDeviceReadIOps))
				assert.Equal(tt, uint64(readIops), inspect.HostConfig.BlkioDeviceReadIOps[0].Rate)
				assert.Equal(tt, 1, len(inspect.HostConfig.BlkioDeviceWriteIOps))
				assert.Equal(tt, uint64(writeIops), inspect.HostConfig.BlkioDeviceWriteIOps[0].Rate)
			},
		}
	}

	testCase.Run(t)
}

func TestContainerInspectUser(t *testing.T) {
	nerdtest.Setup()
	testCase := &test.Case{
		Description: "Container inspect contains User",
		Require:     nerdtest.Build,
		Setup: func(data test.Data, helpers test.Helpers) {
			dockerfile := fmt.Sprintf(`
FROM %s
RUN groupadd -r test && useradd -r -g test test
USER test
`, testutil.UbuntuImage)

			data.Temp().Save(dockerfile, "Dockerfile")

			helpers.Ensure("build", "-t", data.Identifier(), data.Temp().Path())
			helpers.Ensure("create", "--name", data.Identifier(), "--user", "test", data.Identifier())
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rm", "-f", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("inspect", "--format", "{{.Config.User}}", data.Identifier())
		},
		Expected: test.Expects(0, nil, expect.Equals("test\n")),
	}

	testCase.Run(t)
}

type hostConfigValues struct {
	Driver       string
	ShmSize      int64
	PidMode      string
	GroupAddSize int
	Runtime      string
}
