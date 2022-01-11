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
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/containerd/cgroups"
	"github.com/containerd/containerd/sys"
	"github.com/containerd/continuity/testutil/loopback"
	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
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
	if !info.CPUShares {
		t.Skip("test requires CPUShares")
	}
	if !info.CPUSet {
		t.Skip("test requires CPUSet")
	}
	if !info.PidsLimit {
		t.Skip("test requires PidsLimit")
	}

	const expected = `42000 100000
44040192
42
77
0-1
0
`
	//In CgroupV2 CPUWeight replace CPUShares => weight := 1 + ((shares-2)*9999)/262142
	base.Cmd("run", "--rm", "--cpus", "0.42", "--cpuset-mems", "0", "--memory", "42m", "--pids-limit", "42", "--cpu-shares", "2000", "--cpuset-cpus", "0-1", "-w", "/sys/fs/cgroup", testutil.AlpineImage,
		"cat", "cpu.max", "memory.max", "pids.max", "cpu.weight", "cpuset.cpus", "cpuset.mems").AssertOutExactly(expected)
	base.Cmd("run", "--rm", "--cpu-quota", "42000", "--cpuset-mems", "0", "--cpu-period", "100000", "--memory", "42m", "--pids-limit", "42", "--cpu-shares", "2000", "--cpuset-cpus", "0-1", "-w", "/sys/fs/cgroup", testutil.AlpineImage,
		"cat", "cpu.max", "memory.max", "pids.max", "cpu.weight", "cpuset.cpus", "cpuset.mems").AssertOutExactly(expected)
}

func TestRunDevice(t *testing.T) {
	if os.Geteuid() != 0 || sys.RunningInUserNS() {
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
	defer base.Cmd("rm", "-f", containerName).Run()
	// lo0 is readable but not writable.
	// lo1 is readable and writable
	// lo2 is not accessible.
	base.Cmd("run",
		"-d",
		"--name", containerName,
		"--device", lo[0].Device+":r",
		"--device", lo[1].Device,
		testutil.AlpineImage, "sleep", "infinity").Run()

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
		s               string
		expectedDevPath string
		expectedMode    string
		err             string
	}
	testCases := []testCase{
		{
			s:               "/dev/sda1",
			expectedDevPath: "/dev/sda1",
			expectedMode:    "rwm",
		},
		{
			s:               "/dev/sda2:r",
			expectedDevPath: "/dev/sda2",
			expectedMode:    "r",
		},
		{
			s:               "/dev/sda3:rw",
			expectedDevPath: "/dev/sda3",
			expectedMode:    "rw",
		},
		{
			s:   "sda4",
			err: "not an absolute path",
		},
		{
			s:               "/dev/sda5:/dev/sda5",
			expectedDevPath: "/dev/sda5",
			expectedMode:    "rwm",
		},
		{
			s:   "/dev/sda6:/dev/foo6",
			err: "not supported yet",
		},
		{
			s:   "/dev/sda7:/dev/sda7:rwmx",
			err: "unexpected rune",
		},
	}

	for _, tc := range testCases {
		t.Log(tc.s)
		devPath, mode, err := parseDevice(tc.s)
		if tc.err == "" {
			assert.NilError(t, err)
			assert.Equal(t, tc.expectedDevPath, devPath)
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
	// when bfq io scheduler is used, the io.weight knob is exposed as io.bfq.weight
	base.Cmd("run", "--rm", "--blkio-weight", "300", "-w", "/sys/fs/cgroup", testutil.AlpineImage,
		"cat", "io.bfq.weight").AssertOutExactly("default 300\n")
}
