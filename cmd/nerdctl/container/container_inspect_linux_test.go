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
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/docker/go-connections/nat"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/infoutil"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestContainerInspectContainsPortConfig(t *testing.T) {
	testContainer := testutil.Identifier(t)

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainer).Run()

	base.Cmd("run", "-d", "--name", testContainer, "-p", "8080:80", testutil.NginxAlpineImage).AssertOK()
	inspect := base.InspectContainer(testContainer)
	inspect80TCP := (*inspect.NetworkSettings.Ports)["80/tcp"]
	expected := nat.PortBinding{
		HostIP:   "0.0.0.0",
		HostPort: "8080",
	}
	assert.Equal(base.T, expected, inspect80TCP[0])
}

func TestContainerInspectContainsMounts(t *testing.T) {
	testContainer := testutil.Identifier(t)

	base := testutil.NewBase(t)

	testVolume := testutil.Identifier(t)

	defer base.Cmd("volume", "rm", "-f", testVolume).Run()
	base.Cmd("volume", "create", "--label", "tag=testVolume", testVolume).AssertOK()
	inspectVolume := base.InspectVolume(testVolume)
	namedVolumeSource := inspectVolume.Mountpoint

	defer base.Cmd("rm", "-f", testContainer).Run()
	base.Cmd("run", "-d", "--privileged",
		"--name", testContainer,
		"--network", "none",
		"-v", "/anony-vol",
		"--tmpfs", "/app1:size=64m",
		"--mount", "type=bind,src=/tmp,dst=/app2,ro",
		"--mount", fmt.Sprintf("type=volume,src=%s,dst=/app3,readonly=false", testVolume),
		testutil.NginxAlpineImage).AssertOK()

	inspect := base.InspectContainer(testContainer)
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
				Name:        testVolume,
				Source:      namedVolumeSource,
				Destination: "/app3",
				Driver:      localDriver,
				RW:          true,
			},
		},
	}

	for i := range expected {
		testCase := expected[i]
		t.Logf("test volume[dest=%q]", testCase.dest)

		mountPoint, ok := actual[testCase.dest]
		assert.Assert(base.T, ok)

		assert.Equal(base.T, testCase.mountPoint.Type, mountPoint.Type)
		assert.Equal(base.T, testCase.mountPoint.Driver, mountPoint.Driver)
		assert.Equal(base.T, testCase.mountPoint.RW, mountPoint.RW)
		assert.Equal(base.T, testCase.mountPoint.Destination, mountPoint.Destination)

		if testCase.mountPoint.Source != "" {
			assert.Equal(base.T, testCase.mountPoint.Source, mountPoint.Source)
		}
		if testCase.mountPoint.Name != "" {
			assert.Equal(base.T, testCase.mountPoint.Name, mountPoint.Name)
		}
	}
}

func TestContainerInspectContainsLabel(t *testing.T) {
	t.Parallel()
	testContainer := testutil.Identifier(t)

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainer).Run()

	base.Cmd("run", "-d", "--name", testContainer, "--label", "foo=foo", "--label", "bar=bar", testutil.NginxAlpineImage).AssertOK()
	base.EnsureContainerStarted(testContainer)
	inspect := base.InspectContainer(testContainer)
	lbs := inspect.Config.Labels

	assert.Equal(base.T, "foo", lbs["foo"])
	assert.Equal(base.T, "bar", lbs["bar"])
}

func TestContainerInspectContainsInternalLabel(t *testing.T) {
	testutil.DockerIncompatible(t)
	t.Parallel()
	testContainer := testutil.Identifier(t)

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainer).Run()

	base.Cmd("run", "-d", "--name", testContainer, "--mount", "type=bind,src=/tmp,dst=/app,readonly=false,bind-propagation=rprivate", testutil.NginxAlpineImage).AssertOK()
	base.EnsureContainerStarted(testContainer)
	inspect := base.InspectContainer(testContainer)
	lbs := inspect.Config.Labels

	// TODO: add more internal labels testcases
	labelMount := lbs[labels.Mounts]
	expectedLabelMount := "[{\"Type\":\"bind\",\"Source\":\"/tmp\",\"Destination\":\"/app\",\"Mode\":\"rprivate,rbind\",\"RW\":true,\"Propagation\":\"rprivate\"}]"
	assert.Equal(base.T, expectedLabelMount, labelMount)
}

func TestContainerInspectState(t *testing.T) {
	t.Parallel()
	testContainer := testutil.Identifier(t)
	base := testutil.NewBase(t)

	type testCase struct {
		name, containerName, cmd string
		want                     dockercompat.ContainerState
	}
	// nerdctl: run error produces a nil Task, so the Status is empty because Status comes from Task.
	// docker : run error gives => `Status=created` as  in docker there is no a separation between container and Task.
	errStatus := ""
	if base.Target == testutil.Docker {
		errStatus = "created"
	}
	testCases := []testCase{
		{
			name:          "inspect State with error",
			containerName: fmt.Sprintf("%s-fail", testContainer),
			cmd:           "aa",
			want: dockercompat.ContainerState{
				Error:  "executable file not found in $PATH",
				Status: errStatus,
			},
		},
		{
			name:          "inspect State without error",
			containerName: fmt.Sprintf("%s-success", testContainer),
			cmd:           "ls",
			want: dockercompat.ContainerState{
				Error:  "",
				Status: "exited",
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			defer base.Cmd("rm", "-f", tc.containerName).Run()
			if tc.want.Error != "" {
				base.Cmd("run", "--name", tc.containerName, testutil.AlpineImage, tc.cmd).AssertFail()
			} else {
				base.Cmd("run", "--name", tc.containerName, testutil.AlpineImage, tc.cmd).AssertOK()
			}
			inspect := base.InspectContainer(tc.containerName)
			assert.Assert(t, strings.Contains(inspect.State.Error, tc.want.Error), fmt.Sprintf("expected: %s, actual: %s", tc.want.Error, inspect.State.Error))
			assert.Equal(base.T, inspect.State.Status, tc.want.Status)
		})
	}

}

func TestContainerInspectHostConfig(t *testing.T) {
	testContainer := testutil.Identifier(t)
	if rootlessutil.IsRootless() && infoutil.CgroupsVersion() == "1" {
		t.Skip("test skipped for rootless containers on cgroup v1")
	}

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainer).Run()

	// Run a container with various HostConfig options
	base.Cmd("run", "-d", "--name", testContainer,
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
		"--sysctl", "net.core.somaxconn=1024",
		"--runtime", "io.containerd.runc.v2",
		testutil.AlpineImage, "sleep", "infinity").AssertOK()

	inspect := base.InspectContainer(testContainer)

	assert.Equal(t, "0-1", inspect.HostConfig.CPUSetCPUs)
	assert.Equal(t, "0", inspect.HostConfig.CPUSetMems)
	assert.Equal(t, uint64(1024), inspect.HostConfig.CPUShares)
	assert.Equal(t, int64(100000), inspect.HostConfig.CPUQuota)
	assert.Assert(t, slices.Contains(inspect.HostConfig.GroupAdd, "1000"), "Expected '1000' to be in GroupAdd")
	assert.Assert(t, slices.Contains(inspect.HostConfig.GroupAdd, "2000"), "Expected '2000' to be in GroupAdd")
	expectedExtraHosts := []string{"host1:10.0.0.1", "host2:10.0.0.2"}
	assert.DeepEqual(t, expectedExtraHosts, inspect.HostConfig.ExtraHosts)
	assert.Equal(t, "host", inspect.HostConfig.IpcMode)
	assert.Equal(t, int64(536870912), inspect.HostConfig.Memory)
	assert.Equal(t, int64(1073741824), inspect.HostConfig.MemorySwap)
	assert.Equal(t, true, inspect.HostConfig.ReadonlyRootfs)
	assert.Equal(t, "host", inspect.HostConfig.UTSMode)
	assert.Equal(t, int64(268435456), inspect.HostConfig.ShmSize)
}

func TestContainerInspectHostConfigDefaults(t *testing.T) {
	testContainer := testutil.Identifier(t)

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainer).Run()

	var hc hostConfigValues

	// Hostconfig default values differ with Docker.
	// This is because we directly retrieve the configured values instead of using preset defaults.
	if testutil.GetTarget() == testutil.Docker {
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

	// Run a container without specifying HostConfig options
	base.Cmd("run", "-d", "--name", testContainer, testutil.AlpineImage, "sleep", "infinity").AssertOK()

	inspect := base.InspectContainer(testContainer)
	t.Logf("HostConfig in TestContainerInspectHostConfigDefaults: %+v", inspect.HostConfig)
	assert.Equal(t, "", inspect.HostConfig.CPUSetCPUs)
	assert.Equal(t, "", inspect.HostConfig.CPUSetMems)
	assert.Equal(t, uint16(0), inspect.HostConfig.BlkioWeight)
	assert.Equal(t, 0, len(inspect.HostConfig.BlkioWeightDevice))
	assert.Equal(t, 0, len(inspect.HostConfig.BlkioDeviceReadBps))
	assert.Equal(t, 0, len(inspect.HostConfig.BlkioDeviceReadIOps))
	assert.Equal(t, 0, len(inspect.HostConfig.BlkioDeviceWriteBps))
	assert.Equal(t, 0, len(inspect.HostConfig.BlkioDeviceWriteIOps))
	assert.Equal(t, uint64(0), inspect.HostConfig.CPUShares)
	assert.Equal(t, int64(0), inspect.HostConfig.CPUQuota)
	assert.Equal(t, hc.GroupAddSize, len(inspect.HostConfig.GroupAdd))
	assert.Equal(t, 0, len(inspect.HostConfig.ExtraHosts))
	assert.Equal(t, "private", inspect.HostConfig.IpcMode)
	assert.Equal(t, hc.Driver, inspect.HostConfig.LogConfig.Driver)
	assert.Equal(t, int64(0), inspect.HostConfig.Memory)
	assert.Equal(t, int64(0), inspect.HostConfig.MemorySwap)
	assert.Equal(t, bool(false), inspect.HostConfig.OomKillDisable)
	assert.Equal(t, bool(false), inspect.HostConfig.ReadonlyRootfs)
	assert.Equal(t, "", inspect.HostConfig.UTSMode)
	assert.Equal(t, hc.ShmSize, inspect.HostConfig.ShmSize)
	assert.Equal(t, hc.Runtime, inspect.HostConfig.Runtime)
	assert.Equal(t, 0, len(inspect.HostConfig.Sysctls))
	assert.Equal(t, 0, len(inspect.HostConfig.Devices))
}

func TestContainerInspectHostConfigDNS(t *testing.T) {
	testContainer := testutil.Identifier(t)

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainer).Run()

	// Run a container with DNS options
	base.Cmd("run", "-d", "--name", testContainer,
		"--dns", "8.8.8.8",
		"--dns", "1.1.1.1",
		"--dns-search", "example.com",
		"--dns-search", "test.local",
		"--dns-option", "ndots:5",
		"--dns-option", "timeout:3",
		testutil.AlpineImage, "sleep", "infinity").AssertOK()

	inspect := base.InspectContainer(testContainer)

	// Check DNS servers
	expectedDNSServers := []string{"8.8.8.8", "1.1.1.1"}
	assert.DeepEqual(t, expectedDNSServers, inspect.HostConfig.DNS)

	// Check DNS search domains
	expectedDNSSearch := []string{"example.com", "test.local"}
	assert.DeepEqual(t, expectedDNSSearch, inspect.HostConfig.DNSSearch)

	// Check DNS options
	expectedDNSOptions := []string{"ndots:5", "timeout:3"}
	assert.DeepEqual(t, expectedDNSOptions, inspect.HostConfig.DNSOptions)
}

func TestContainerInspectHostConfigDNSDefaults(t *testing.T) {
	testContainer := testutil.Identifier(t)

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainer).Run()

	// Run a container without specifying DNS options
	base.Cmd("run", "-d", "--name", testContainer, testutil.AlpineImage, "sleep", "infinity").AssertOK()

	inspect := base.InspectContainer(testContainer)

	// Check that DNS settings are empty by default
	assert.Equal(t, 0, len(inspect.HostConfig.DNS))
	assert.Equal(t, 0, len(inspect.HostConfig.DNSSearch))
	assert.Equal(t, 0, len(inspect.HostConfig.DNSOptions))
}

func TestContainerInspectHostConfigPID(t *testing.T) {
	testContainer1 := testutil.Identifier(t) + "-container1"
	testContainer2 := testutil.Identifier(t) + "-container2"

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainer1, testContainer2).Run()

	// Run the first container
	base.Cmd("run", "-d", "--name", testContainer1, testutil.AlpineImage, "sleep", "infinity").AssertOK()

	containerID1 := strings.TrimSpace(base.Cmd("inspect", "-f", "{{.Id}}", testContainer1).Out())

	var hc hostConfigValues

	if testutil.GetTarget() == testutil.Docker {
		hc.PidMode = "container:" + containerID1
	} else {
		hc.PidMode = containerID1
	}

	base.Cmd("run", "-d", "--name", testContainer2,
		"--pid", fmt.Sprintf("container:%s", testContainer1),
		testutil.AlpineImage, "sleep", "infinity").AssertOK()

	inspect := base.InspectContainer(testContainer2)

	assert.Equal(t, hc.PidMode, inspect.HostConfig.PidMode)

}

func TestContainerInspectHostConfigPIDDefaults(t *testing.T) {
	testContainer := testutil.Identifier(t)

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainer).Run()

	base.Cmd("run", "-d", "--name", testContainer, testutil.AlpineImage, "sleep", "infinity").AssertOK()

	inspect := base.InspectContainer(testContainer)

	assert.Equal(t, "", inspect.HostConfig.PidMode)
}

func TestContainerInspectDevices(t *testing.T) {
	testContainer := testutil.Identifier(t)

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainer).Run()

	if rootlessutil.IsRootless() && infoutil.CgroupsVersion() == "1" {
		t.Skip("test skipped for rootless containers on cgroup v1")
	}

	// Create a temporary directory
	dir, err := os.MkdirTemp(t.TempDir(), "device-dir")
	if err != nil {
		t.Fatal(err)
	}

	if testutil.GetTarget() == testutil.Docker {
		dir = "/dev/zero"
	}

	// Run the container with the directory mapped as a device
	base.Cmd("run", "-d", "--name", testContainer,
		"--device", dir+":/dev/xvda",
		testutil.AlpineImage, "sleep", "infinity").AssertOK()

	inspect := base.InspectContainer(testContainer)

	expectedDevices := []dockercompat.DeviceMapping{
		{
			PathOnHost:        dir,
			PathInContainer:   "/dev/xvda",
			CgroupPermissions: "rwm",
		},
	}
	assert.DeepEqual(t, expectedDevices, inspect.HostConfig.Devices)
}

func TestContainerInspectBlkioSettings(t *testing.T) {
	testutil.DockerIncompatible(t)
	testContainer := testutil.Identifier(t)
	// Some of the blkio settings are not supported in cgroup v1.
	// So skip this test if running on cgroup v1
	if infoutil.CgroupsVersion() == "1" {
		t.Skip("test skipped for rootless containers or if running with cgroup v1")
	}

	if rootlessutil.IsRootless() {
		t.Skip("test requires root privilege to create a dummy device")
	}

	devPath := "/dev/dummy-zero"
	// a dummy zero device: mknod /dev/dummy-zero c 1 5
	helperCmd := exec.Command("mknod", []string{devPath, "c", "1", "5"}...)
	if out, err := helperCmd.CombinedOutput(); err != nil {
		err = fmt.Errorf("cannot create %q: %q: %w", devPath, string(out), err)
		t.Fatal(err)
	}

	// ensure the file will be removed in case of failed in the test
	defer func() {
		if err := exec.Command("rm", "-f", devPath).Run(); err != nil {
			t.Logf("failed to remove device %s: %v", devPath, err)
		}
	}()

	base := testutil.NewBase(t)
	defer base.Cmd("rm", "-f", testContainer).AssertOK()

	base.Cmd("run", "-d", "--name", testContainer,
		"--blkio-weight", "500",
		"--blkio-weight-device", "/dev/dummy-zero:500",
		"--device-read-bps", "/dev/dummy-zero:1048576",
		"--device-read-iops", "/dev/dummy-zero:1000",
		"--device-write-bps", "/dev/dummy-zero:2097152",
		"--device-write-iops", "/dev/dummy-zero:2000",
		testutil.AlpineImage, "sleep", "infinity").AssertOK()

	inspect := base.InspectContainer(testContainer)
	assert.Equal(t, uint16(500), inspect.HostConfig.BlkioWeight)
	assert.Equal(t, 1, len(inspect.HostConfig.BlkioWeightDevice))
	assert.Equal(t, uint16(500), *inspect.HostConfig.BlkioWeightDevice[0].Weight)
	assert.Equal(t, 1, len(inspect.HostConfig.BlkioDeviceReadBps))
	assert.Equal(t, uint64(1048576), inspect.HostConfig.BlkioDeviceReadBps[0].Rate)
	assert.Equal(t, 1, len(inspect.HostConfig.BlkioDeviceWriteBps))
	assert.Equal(t, uint64(2097152), inspect.HostConfig.BlkioDeviceWriteBps[0].Rate)
	assert.Equal(t, 1, len(inspect.HostConfig.BlkioDeviceReadIOps))
	assert.Equal(t, uint64(1000), inspect.HostConfig.BlkioDeviceReadIOps[0].Rate)
	assert.Equal(t, 1, len(inspect.HostConfig.BlkioDeviceWriteIOps))
	assert.Equal(t, uint64(2000), inspect.HostConfig.BlkioDeviceWriteIOps[0].Rate)
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

			err := os.WriteFile(filepath.Join(data.Temp().Path(), "Dockerfile"), []byte(dockerfile), 0o600)
			assert.NilError(helpers.T(), err)

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
