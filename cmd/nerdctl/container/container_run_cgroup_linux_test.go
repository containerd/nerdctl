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
	"path/filepath"
	"testing"

	"github.com/moby/sys/userns"
	"gotest.tools/v3/assert"

	"github.com/containerd/cgroups/v3"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/continuity/testutil/loopback"

	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
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
	testCase.Require = test.Not(nerdtest.Docker)

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
	if os.Geteuid() != 0 || userns.RunningInUserNS() {
		t.Skip("test requires the root in the initial user namespace")
	}

	const n = 3
	lo := make([]*loopback.Loopback, n)
	loContent := make([]string, n)

	for i := 0; i < n; i++ {
		var err error
		lo[i], err = loopback.New(4096)
		assert.NilError(t, err)
		t.Logf("lo[%d] = %+v", i, lo[i])
		defer lo[i].Close()
		loContent[i] = fmt.Sprintf("lo%d-content", i)
		assert.NilError(t, os.WriteFile(lo[i].Device, []byte(loContent[i]), 0700))
	}

	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", containerName).AssertOK()
	// lo0 is readable but not writable.
	// lo1 is readable and writable
	// lo2 is not accessible.
	base.Cmd("run",
		"-d",
		"--name", containerName,
		"--device", lo[0].Device+":r",
		"--device", lo[1].Device,
		testutil.AlpineImage, "sleep", nerdtest.Infinity).Run()

	base.Cmd("exec", containerName, "cat", lo[0].Device).AssertOutContains(loContent[0])
	base.Cmd("exec", containerName, "cat", lo[1].Device).AssertOutContains(loContent[1])
	base.Cmd("exec", containerName, "cat", lo[2].Device).AssertFail()
	base.Cmd("exec", containerName, "sh", "-ec", "echo -n \"overwritten-lo0-content\">"+lo[0].Device).AssertFail()
	base.Cmd("exec", containerName, "sh", "-ec", "echo -n \"overwritten-lo1-content\">"+lo[1].Device).AssertOK()
	lo1Read, err := os.ReadFile(lo[1].Device)
	assert.NilError(t, err)
	assert.Equal(t, string(bytes.Trim(lo1Read, "\x00")), "overwritten-lo1-content")
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
