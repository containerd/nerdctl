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
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/cgroups/v3"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/continuity/testutil/loopback"
	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestRunCgroupV2(t *testing.T) {
	t.Parallel()
	if cgroups.Mode() != cgroups.Unified {
		t.Skip("test requires cgroup v2")
	}
	base := testutil.NewBase(t)
	info := base.Info()
	switch info.CgroupDriver {
	case "none", "":
		t.Skip("test requires cgroup driver")
	}

	if !info.MemoryLimit {
		t.Skip("test requires MemoryLimit")
	}
	if !info.SwapLimit {
		t.Skip("test requires SwapLimit")
	}
	if !info.CPUShares {
		t.Skip("test requires CPUShares")
	}
	if !info.CPUSet {
		t.Skip("test requires CPUSet")
	}
	if !info.PidsLimit {
		t.Skip("test requires PidsLimit")
	}
	const expected1 = `42000 100000
44040192
44040192
42
77
0-1
0
`
	const expected2 = `42000 100000
44040192
60817408
6291456
42
77
0-1
0
`

	// In CgroupV2 CPUWeight replace CPUShares => weight := 1 + ((shares-2)*9999)/262142
	base.Cmd("run", "--rm",
		"--cpus", "0.42", "--cpuset-mems", "0",
		"--memory", "42m",
		"--pids-limit", "42",
		"--cpu-shares", "2000", "--cpuset-cpus", "0-1",
		"-w", "/sys/fs/cgroup", testutil.AlpineImage,
		"cat", "cpu.max", "memory.max", "memory.swap.max",
		"pids.max", "cpu.weight", "cpuset.cpus", "cpuset.mems").AssertOutExactly(expected1)
	base.Cmd("run", "--rm",
		"--cpu-quota", "42000", "--cpuset-mems", "0",
		"--cpu-period", "100000", "--memory", "42m", "--memory-reservation", "6m", "--memory-swap", "100m",
		"--pids-limit", "42", "--cpu-shares", "2000", "--cpuset-cpus", "0-1",
		"-w", "/sys/fs/cgroup", testutil.AlpineImage,
		"cat", "cpu.max", "memory.max", "memory.swap.max", "memory.low", "pids.max",
		"cpu.weight", "cpuset.cpus", "cpuset.mems").AssertOutExactly(expected2)

	base.Cmd("run", "--name", testutil.Identifier(t)+"-testUpdate1", "-w", "/sys/fs/cgroup", "-d",
		testutil.AlpineImage, "sleep", nerdtest.Infinity).AssertOK()
	defer base.Cmd("rm", "-f", testutil.Identifier(t)+"-testUpdate1").Run()
	update := []string{"update", "--cpu-quota", "42000", "--cpuset-mems", "0", "--cpu-period", "100000",
		"--memory", "42m",
		"--pids-limit", "42", "--cpu-shares", "2000", "--cpuset-cpus", "0-1"}
	if base.Target == testutil.Docker && info.CgroupVersion == "2" && info.SwapLimit {
		// Workaround for Docker with cgroup v2:
		// > Error response from daemon: Cannot update container 67c13276a13dd6a091cdfdebb355aa4e1ecb15fbf39c2b5c9abee89053e88fce:
		// > Memory limit should be smaller than already set memoryswap limit, update the memoryswap at the same time
		update = append(update, "--memory-swap=84m")
	}
	update = append(update, testutil.Identifier(t)+"-testUpdate1")
	base.Cmd(update...).AssertOK()
	base.Cmd("exec", testutil.Identifier(t)+"-testUpdate1",
		"cat", "cpu.max", "memory.max", "memory.swap.max",
		"pids.max", "cpu.weight", "cpuset.cpus", "cpuset.mems").AssertOutExactly(expected1)

	defer base.Cmd("rm", "-f", testutil.Identifier(t)+"-testUpdate2").Run()
	base.Cmd("run", "--name", testutil.Identifier(t)+"-testUpdate2", "-w", "/sys/fs/cgroup", "-d",
		testutil.AlpineImage, "sleep", nerdtest.Infinity).AssertOK()
	base.EnsureContainerStarted(testutil.Identifier(t) + "-testUpdate2")

	base.Cmd("update", "--cpu-quota", "42000", "--cpuset-mems", "0", "--cpu-period", "100000",
		"--memory", "42m", "--memory-reservation", "6m", "--memory-swap", "100m",
		"--pids-limit", "42", "--cpu-shares", "2000", "--cpuset-cpus", "0-1",
		testutil.Identifier(t)+"-testUpdate2").AssertOK()
	base.Cmd("exec", testutil.Identifier(t)+"-testUpdate2",
		"cat", "cpu.max", "memory.max", "memory.swap.max", "memory.low",
		"pids.max", "cpu.weight", "cpuset.cpus", "cpuset.mems").AssertOutExactly(expected2)

}

func TestRunCgroupV1(t *testing.T) {
	t.Parallel()
	switch cgroups.Mode() {
	case cgroups.Legacy, cgroups.Hybrid:
	default:
		t.Skip("test requires cgroup v1")
	}
	base := testutil.NewBase(t)
	info := base.Info()
	switch info.CgroupDriver {
	case "none", "":
		t.Skip("test requires cgroup driver")
	}
	if !info.MemoryLimit {
		t.Skip("test requires MemoryLimit")
	}
	if !info.CPUShares {
		t.Skip("test requires CPUShares")
	}
	if !info.CPUSet {
		t.Skip("test requires CPUSet")
	}
	if !info.PidsLimit {
		t.Skip("test requires PidsLimit")
	}
	quota := "/sys/fs/cgroup/cpu/cpu.cfs_quota_us"
	period := "/sys/fs/cgroup/cpu/cpu.cfs_period_us"
	cpusetMems := "/sys/fs/cgroup/cpuset/cpuset.mems"
	memoryLimit := "/sys/fs/cgroup/memory/memory.limit_in_bytes"
	memoryReservation := "/sys/fs/cgroup/memory/memory.soft_limit_in_bytes"
	memorySwap := "/sys/fs/cgroup/memory/memory.memsw.limit_in_bytes"
	memorySwappiness := "/sys/fs/cgroup/memory/memory.swappiness"
	pidsLimit := "/sys/fs/cgroup/pids/pids.max"
	cpuShare := "/sys/fs/cgroup/cpu/cpu.shares"
	cpusetCpus := "/sys/fs/cgroup/cpuset/cpuset.cpus"

	const expected = "42000\n100000\n0\n44040192\n6291456\n104857600\n0\n42\n2000\n0-1\n"
	base.Cmd("run", "--rm", "--cpus", "0.42", "--cpuset-mems", "0", "--memory", "42m", "--memory-reservation", "6m", "--memory-swap", "100m", "--memory-swappiness", "0", "--pids-limit", "42", "--cpu-shares", "2000", "--cpuset-cpus", "0-1", testutil.AlpineImage, "cat", quota, period, cpusetMems, memoryLimit, memoryReservation, memorySwap, memorySwappiness, pidsLimit, cpuShare, cpusetCpus).AssertOutExactly(expected)
	base.Cmd("run", "--rm", "--cpu-quota", "42000", "--cpu-period", "100000", "--cpuset-mems", "0", "--memory", "42m", "--memory-reservation", "6m", "--memory-swap", "100m", "--memory-swappiness", "0", "--pids-limit", "42", "--cpu-shares", "2000", "--cpuset-cpus", "0-1", testutil.AlpineImage, "cat", quota, period, cpusetMems, memoryLimit, memoryReservation, memorySwap, memorySwappiness, pidsLimit, cpuShare, cpusetCpus).AssertOutExactly(expected)
}

// TestIssue3781 tests https://github.com/containerd/nerdctl/issues/3781
func TestIssue3781(t *testing.T) {
	t.Parallel()
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)

	base := testutil.NewBase(t)
	info := base.Info()
	switch info.CgroupDriver {
	case "none", "":
		t.Skip("test requires cgroup driver")
	}
	containerName := testutil.Identifier(t)
	base.Cmd("run", "-d", "--name", containerName, testutil.AlpineImage, "sleep", "infinity").AssertOK()
	defer func() {
		base.Cmd("rm", "-f", containerName)
	}()
	base.Cmd("update", "--cpuset-cpus", "0-1", containerName).AssertOK()
	addr := base.ContainerdAddress()
	client, err := containerd.New(addr, containerd.WithDefaultNamespace(testutil.Namespace))
	assert.NilError(base.T, err)
	ctx := context.Background()

	// get container id by container name.
	var cid string
	var args []string
	args = append(args, containerName)
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			cid = found.Container.ID()
			return nil
		},
	}
	err = walker.WalkAll(ctx, args, true)
	assert.NilError(base.T, err)

	container, err := client.LoadContainer(ctx, cid)
	assert.NilError(base.T, err)
	spec, err := container.Spec(ctx)
	assert.NilError(base.T, err)
	assert.Equal(t, spec.Linux.Resources.Pids == nil, true)
}

func TestRunDevice(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.Rootful

	const n = 3
	lo := make([]*loopback.Loopback, n)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {

		for i := 0; i < n; i++ {
			var err error
			lo[i], err = loopback.New(4096)
			assert.NilError(t, err)
			t.Logf("lo[%d] = %+v", i, lo[i])
			loContent := fmt.Sprintf("lo%d-content", i)
			assert.NilError(t, os.WriteFile(lo[i].Device, []byte(loContent), 0o700))
			data.Labels().Set("loContent"+strconv.Itoa(i), loContent)
		}

		// lo0 is readable but not writable.
		// lo1 is readable and writable
		// lo2 is not accessible.
		helpers.Ensure("run",
			"-d",
			"--name", data.Identifier(),
			"--device", lo[0].Device+":r",
			"--device", lo[1].Device,
			testutil.AlpineImage, "sleep", nerdtest.Infinity)
		data.Labels().Set("id", data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		for i := 0; i < n; i++ {
			if lo[i] != nil {
				_ = lo[i].Close()
			}
		}
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "can read lo0",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Labels().Get("id"), "cat", lo[0].Device)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.Contains(data.Labels().Get("locontent0")),
				}
			},
		},
		{
			Description: "cannot write lo0",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Labels().Get("id"), "sh", "-ec", "echo -n \"overwritten-lo1-content\">"+lo[0].Device)
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "cannot read lo2",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Labels().Get("id"), "cat", lo[2].Device)
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "can read lo1",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Labels().Get("id"), "cat", lo[1].Device)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.Contains(data.Labels().Get("locontent1")),
				}
			},
		},
		{
			Description: "can write lo1 and read back updated value",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Labels().Get("id"), "sh", "-ec", "echo -n \"overwritten-lo1-content\">"+lo[1].Device)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, info string, t *testing.T) {
				lo1Read, err := os.ReadFile(lo[1].Device)
				assert.NilError(t, err)
				assert.Equal(t, string(bytes.Trim(lo1Read, "\x00")), "overwritten-lo1-content")
			}),
		},
	}

	testCase.Run(t)
}

func TestParseDevice(t *testing.T) {
	t.Parallel()
	type testCase struct {
		s                     string
		expectedDevPath       string
		expectedContainerPath string
		expectedMode          string
		err                   string
	}
	testCases := []testCase{
		{
			s:                     "/dev/sda1",
			expectedDevPath:       "/dev/sda1",
			expectedContainerPath: "/dev/sda1",
			expectedMode:          "rwm",
		},
		{
			s:                     "/dev/sda2:r",
			expectedDevPath:       "/dev/sda2",
			expectedContainerPath: "/dev/sda2",
			expectedMode:          "r",
		},
		{
			s:                     "/dev/sda3:rw",
			expectedDevPath:       "/dev/sda3",
			expectedContainerPath: "/dev/sda3",
			expectedMode:          "rw",
		},
		{
			s:   "sda4",
			err: "not an absolute path",
		},
		{
			s:                     "/dev/sda5:/dev/sda5",
			expectedDevPath:       "/dev/sda5",
			expectedContainerPath: "/dev/sda5",
			expectedMode:          "rwm",
		},
		{
			s:                     "/dev/sda6:/dev/foo6",
			expectedDevPath:       "/dev/sda6",
			expectedContainerPath: "/dev/foo6",
			expectedMode:          "rwm",
		},
		{
			s:   "/dev/sda7:/dev/sda7:rwmx",
			err: "unexpected rune",
		},
	}

	for _, tc := range testCases {
		t.Log(tc.s)
		devPath, containerPath, mode, err := container.ParseDevice(tc.s)
		if tc.err == "" {
			assert.NilError(t, err)
			assert.Equal(t, tc.expectedDevPath, devPath)
			assert.Equal(t, tc.expectedContainerPath, containerPath)
			assert.Equal(t, tc.expectedMode, mode)
		} else {
			assert.ErrorContains(t, err, tc.err)
		}
	}
}

func TestRunCgroupConf(t *testing.T) {
	t.Parallel()
	if cgroups.Mode() != cgroups.Unified {
		t.Skip("test requires cgroup v2")
	}
	testutil.DockerIncompatible(t) // Docker lacks --cgroup-conf
	base := testutil.NewBase(t)
	info := base.Info()
	switch info.CgroupDriver {
	case "none", "":
		t.Skip("test requires cgroup driver")
	}
	if !info.MemoryLimit {
		t.Skip("test requires MemoryLimit")
	}
	base.Cmd("run", "--rm", "--cgroup-conf", "memory.high=33554432", "-w", "/sys/fs/cgroup", testutil.AlpineImage,
		"cat", "memory.high").AssertOutExactly("33554432\n")
}

func TestRunCgroupParent(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	info := base.Info()
	switch info.CgroupDriver {
	case "none", "":
		t.Skip("test requires cgroup driver")
	}

	containerName := testutil.Identifier(t)
	t.Logf("Using %q cgroup driver", info.CgroupDriver)

	parent := "/foobarbaz"
	if info.CgroupDriver == "systemd" {
		// Path separators aren't allowed in systemd path. runc
		// explicitly checks for this.
		// https://github.com/opencontainers/runc/blob/016a0d29d1750180b2a619fc70d6fe0d80111be0/libcontainer/cgroups/systemd/common.go#L65-L68
		parent = "foobarbaz.slice"
	}

	tearDown := func() {
		base.Cmd("rm", "-f", containerName).Run()
	}

	tearDown()
	t.Cleanup(tearDown)

	// cgroup2 without host cgroup ns will just output 0::/ which doesn't help much to verify
	// we got our expected path. This approach should work for both cgroup1 and 2, there will
	// just be many more entries for cgroup1 as there'll be an entry per controller.
	base.Cmd(
		"run",
		"-d",
		"--name",
		containerName,
		"--cgroupns=host",
		"--cgroup-parent", parent,
		testutil.AlpineImage,
		"sleep",
		"infinity",
	).AssertOK()

	id := base.InspectContainer(containerName).ID
	expected := filepath.Join(parent, id)
	if info.CgroupDriver == "systemd" {
		expected = filepath.Join(parent, fmt.Sprintf("nerdctl-%s", id))
		if base.Target == testutil.Docker {
			expected = filepath.Join(parent, fmt.Sprintf("docker-%s", id))
		}
	}
	base.Cmd("exec", containerName, "cat", "/proc/self/cgroup").AssertOutContains(expected)
}

func TestRunBlkioWeightCgroupV2(t *testing.T) {
	t.Parallel()
	if cgroups.Mode() != cgroups.Unified {
		t.Skip("test requires cgroup v2")
	}
	if _, err := os.Stat("/sys/module/bfq"); err != nil {
		t.Skipf("test requires \"bfq\" module to be loaded: %v", err)
	}
	base := testutil.NewBase(t)
	info := base.Info()
	switch info.CgroupDriver {
	case "none", "":
		t.Skip("test requires cgroup driver")
	}
	containerName := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", containerName).AssertOK()
	// when bfq io scheduler is used, the io.weight knob is exposed as io.bfq.weight
	base.Cmd("run", "--name", containerName, "--blkio-weight", "300", "-w", "/sys/fs/cgroup", testutil.AlpineImage, "sleep", nerdtest.Infinity).AssertOK()
	base.Cmd("exec", containerName, "cat", "io.bfq.weight").AssertOutExactly("default 300\n")
	base.Cmd("update", containerName, "--blkio-weight", "400").AssertOK()
	base.Cmd("exec", containerName, "cat", "io.bfq.weight").AssertOutExactly("default 400\n")
}

func TestRunBlkioSettingCgroupV2(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Rootful

	// Create dummy device path
	dummyDev := "/dev/dummy-zero"

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// Create dummy device
		helperCmd := exec.Command("mknod", dummyDev, "c", "1", "5")
		if out, err := helperCmd.CombinedOutput(); err != nil {
			t.Fatalf("cannot create %q: %q: %v", dummyDev, string(out), err)
		}
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		// Clean up the dummy device
		if err := exec.Command("rm", "-f", dummyDev).Run(); err != nil {
			t.Logf("failed to remove device %s: %v", dummyDev, err)
		}
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "blkio-weight",
			Require:     nerdtest.CGroupV2,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "-d", "--name", data.Identifier(),
					"--blkio-weight", "150",
					testutil.AlpineImage, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(
						func(stdout string, info string, t *testing.T) {
							assert.Assert(t, strings.Contains(helpers.Capture("inspect", "--format", "{{.HostConfig.BlkioWeight}}", data.Identifier()), "150"))
						},
					),
				}
			},
		},
		{
			Description: "blkio-weight-device",
			Require:     nerdtest.CGroupV2,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "-d", "--name", data.Identifier(),
					"--blkio-weight-device", dummyDev+":100",
					testutil.AlpineImage, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(
						func(stdout string, info string, t *testing.T) {
							inspectOut := helpers.Capture("inspect", "--format", "{{range .HostConfig.BlkioWeightDevice}}{{.Weight}}{{end}}", data.Identifier())
							assert.Assert(t, strings.Contains(inspectOut, "100"))
						},
					),
				}
			},
		},
		{
			Description: "device-read-bps",
			Require: require.All(
				nerdtest.CGroupV2,
				// Docker cli (v26.1.3) available in github runners has a bug where some of the blkio options
				// do not work https://github.com/docker/cli/issues/5321. The fix has been merged to the latest releases
				// but not currently available in the v26 release.
				require.Not(nerdtest.Docker),
			),
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "-d", "--name", data.Identifier(),
					"--device-read-bps", dummyDev+":1048576",
					testutil.AlpineImage, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(
						func(stdout string, info string, t *testing.T) {
							inspectOut := helpers.Capture("inspect", "--format", "{{range .HostConfig.BlkioDeviceReadBps}}{{.Rate}}{{end}}", data.Identifier())
							assert.Assert(t, strings.Contains(inspectOut, "1048576"))
						},
					),
				}
			},
		},
		{
			Description: "device-write-bps",
			Require: require.All(
				nerdtest.CGroupV2,
				// Docker cli (v26.1.3) available in github runners has a bug where some of the blkio options
				// do not work https://github.com/docker/cli/issues/5321. The fix has been merged to the latest releases
				// but not currently available in the v26 release.
				require.Not(nerdtest.Docker),
			),
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "-d", "--name", data.Identifier(),
					"--device-write-bps", dummyDev+":2097152",
					testutil.AlpineImage, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(
						func(stdout string, info string, t *testing.T) {
							inspectOut := helpers.Capture("inspect", "--format", "{{range .HostConfig.BlkioDeviceWriteBps}}{{.Rate}}{{end}}", data.Identifier())
							assert.Assert(t, strings.Contains(inspectOut, "2097152"))
						},
					),
				}
			},
		},
		{
			Description: "device-read-iops",
			Require: require.All(
				nerdtest.CGroupV2,
				// Docker cli (v26.1.3) available in github runners has a bug where some of the blkio options
				// do not work https://github.com/docker/cli/issues/5321. The fix has been merged to the latest releases
				// but not currently available in the v26 release.
				require.Not(nerdtest.Docker),
			),
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "-d", "--name", data.Identifier(),
					"--device-read-iops", dummyDev+":1000",
					testutil.AlpineImage, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(
						func(stdout string, info string, t *testing.T) {
							inspectOut := helpers.Capture("inspect", "--format", "{{range .HostConfig.BlkioDeviceReadIOps}}{{.Rate}}{{end}}", data.Identifier())
							assert.Assert(t, strings.Contains(inspectOut, "1000"))
						},
					),
				}
			},
		},
		{
			Description: "device-write-iops",
			Require: require.All(
				nerdtest.CGroupV2,
				// Docker cli (v26.1.3) available in github runners has a bug where some of the blkio options
				// do not work https://github.com/docker/cli/issues/5321. The fix has been merged to the latest releases
				// but not currently available in the v26 release.
				require.Not(nerdtest.Docker),
			),
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "-d", "--name", data.Identifier(),
					"--device-write-iops", dummyDev+":2000",
					testutil.AlpineImage, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(
						func(stdout string, info string, t *testing.T) {
							inspectOut := helpers.Capture("inspect", "--format", "{{range .HostConfig.BlkioDeviceWriteIOps}}{{.Rate}}{{end}}", data.Identifier())
							assert.Assert(t, strings.Contains(inspectOut, "2000"))
						},
					),
				}
			},
		},
	}

	testCase.Run(t)
}

func TestRunCPURealTimeSettingCgroupV1(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Description: "cpu-rt-runtime-and-period",
		Require: require.All(
			require.Not(nerdtest.CGroupV2),
			nerdtest.Rootful,
		),
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("create", "--name", data.Identifier(),
				"--cpu-rt-runtime", "950000",
				"--cpu-rt-period", "1000000",
				testutil.AlpineImage, "sleep", "infinity")
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rm", "-f", data.Identifier())
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				ExitCode: 0,
				Output: expect.All(
					func(stdout string, info string, t *testing.T) {
						rtRuntime := helpers.Capture("inspect", "--format", "{{.HostConfig.CPURealtimeRuntime}}", data.Identifier())
						rtPeriod := helpers.Capture("inspect", "--format", "{{.HostConfig.CPURealtimePeriod}}", data.Identifier())
						assert.Assert(t, strings.Contains(rtRuntime, "950000"))
						assert.Assert(t, strings.Contains(rtPeriod, "1000000"))
					},
				),
			}
		},
	}

	testCase.Run(t)
}
