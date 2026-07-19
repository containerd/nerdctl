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
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/defaults"
	"github.com/containerd/continuity/testutil/loopback"
	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestRunCgroupV2(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(
		nerdtest.CGroupV2,
		nerdtest.Info(func(info dockercompat.Info) error {
			if info.CgroupDriver == "none" || info.CgroupDriver == "" {
				return fmt.Errorf("test requires cgroup driver")
			}
			if !info.MemoryLimit {
				return fmt.Errorf("test requires MemoryLimit")
			}
			if !info.SwapLimit {
				return fmt.Errorf("test requires SwapLimit")
			}
			if !info.CPUSet {
				return fmt.Errorf("test requires CPUSet")
			}
			if !info.PidsLimit {
				return fmt.Errorf("test requires PidsLimit")
			}
			return nil
		}),
	)

	const expected1 = `42000 100000
44040192
44040192
42
0-1
0
`
	const expected2 = `42000 100000
44040192
60817408
6291456
42
0-1
0
`

	testCase.SubTests = []*test.Case{
		{
			Description: "cpus and memory",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm",
					"--cpus", "0.42", "--cpuset-mems", "0",
					"--memory", "42m",
					"--pids-limit", "42",
					"--cpuset-cpus", "0-1",
					"-w", "/sys/fs/cgroup", testutil.AlpineImage,
					"cat", "cpu.max", "memory.max", "memory.swap.max",
					"pids.max", "cpuset.cpus", "cpuset.mems")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.Equals(expected1),
				}
			},
		},
		{
			Description: "explicit quota and period",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm",
					"--cpu-quota", "42000", "--cpuset-mems", "0",
					"--cpu-period", "100000", "--memory", "42m", "--memory-reservation", "6m", "--memory-swap", "100m",
					"--pids-limit", "42", "--cpuset-cpus", "0-1",
					"-w", "/sys/fs/cgroup", testutil.AlpineImage,
					"cat", "cpu.max", "memory.max", "memory.swap.max", "memory.low", "pids.max",
					"cpuset.cpus", "cpuset.mems")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.Equals(expected2),
				}
			},
		},
		{
			Description: "update basic",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--name", data.Identifier(), "-w", "/sys/fs/cgroup", "-d",
					testutil.AlpineImage, "sleep", nerdtest.Infinity)
				update := []string{"update", "--cpu-quota", "42000", "--cpuset-mems", "0", "--cpu-period", "100000",
					"--memory", "42m",
					"--pids-limit", "42", "--cpuset-cpus", "0-1"}
				if nerdtest.IsDocker() {
					// Workaround for Docker with cgroup v2:
					// > Error response from daemon: Cannot update container ...:
					// > Memory limit should be smaller than already set memoryswap limit, update the memoryswap at the same time
					update = append(update, "--memory-swap=84m")
				}
				update = append(update, data.Identifier())
				helpers.Ensure(update...)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Identifier(),
					"cat", "cpu.max", "memory.max", "memory.swap.max",
					"pids.max", "cpuset.cpus", "cpuset.mems")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.Equals(expected1),
				}
			},
		},
		{
			Description: "update with reservation and swap",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "--name", data.Identifier(), "-w", "/sys/fs/cgroup", "-d",
					testutil.AlpineImage, "sleep", nerdtest.Infinity)
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
				helpers.Ensure("update", "--cpu-quota", "42000", "--cpuset-mems", "0", "--cpu-period", "100000",
					"--memory", "42m", "--memory-reservation", "6m", "--memory-swap", "100m",
					"--pids-limit", "42", "--cpuset-cpus", "0-1",
					data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Identifier(),
					"cat", "cpu.max", "memory.max", "memory.swap.max", "memory.low",
					"pids.max", "cpuset.cpus", "cpuset.mems")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.Equals(expected2),
				}
			},
		},
		{
			Description: "writable-cgroups true",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--security-opt", "writable-cgroups=true", testutil.AlpineImage, "mkdir", "/sys/fs/cgroup/foo")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "writable-cgroups false",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--security-opt", "writable-cgroups=false", testutil.AlpineImage, "mkdir", "/sys/fs/cgroup/foo")
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "writable-cgroups default",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", testutil.AlpineImage, "mkdir", "/sys/fs/cgroup/foo")
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
	}
	testCase.Run(t)
}

func TestRunCgroupV1(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(
		require.Not(nerdtest.CGroupV2),
		nerdtest.Info(func(info dockercompat.Info) error {
			if info.CgroupDriver == "none" || info.CgroupDriver == "" {
				return fmt.Errorf("test requires cgroup driver")
			}
			if !info.MemoryLimit {
				return fmt.Errorf("test requires MemoryLimit")
			}
			if !info.CPUShares {
				return fmt.Errorf("test requires CPUShares")
			}
			if !info.CPUSet {
				return fmt.Errorf("test requires CPUSet")
			}
			if !info.PidsLimit {
				return fmt.Errorf("test requires PidsLimit")
			}
			return nil
		}),
	)

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

	testCase.SubTests = []*test.Case{
		{
			Description: "cpus and memory",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--cpus", "0.42", "--cpuset-mems", "0", "--memory", "42m", "--memory-reservation", "6m", "--memory-swap", "100m", "--memory-swappiness", "0", "--pids-limit", "42", "--cpu-shares", "2000", "--cpuset-cpus", "0-1", testutil.AlpineImage, "cat", quota, period, cpusetMems, memoryLimit, memoryReservation, memorySwap, memorySwappiness, pidsLimit, cpuShare, cpusetCpus)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.Equals(expected),
				}
			},
		},
		{
			Description: "explicit quota and period",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--cpu-quota", "42000", "--cpu-period", "100000", "--cpuset-mems", "0", "--memory", "42m", "--memory-reservation", "6m", "--memory-swap", "100m", "--memory-swappiness", "0", "--pids-limit", "42", "--cpu-shares", "2000", "--cpuset-cpus", "0-1", testutil.AlpineImage, "cat", quota, period, cpusetMems, memoryLimit, memoryReservation, memorySwap, memorySwappiness, pidsLimit, cpuShare, cpusetCpus)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.Equals(expected),
				}
			},
		},
		{
			Description: "writable-cgroups true",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--security-opt", "writable-cgroups=true", testutil.AlpineImage, "mkdir", "/sys/fs/cgroup/pids/foo")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "writable-cgroups false",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--security-opt", "writable-cgroups=false", testutil.AlpineImage, "mkdir", "/sys/fs/cgroup/pids/foo")
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "writable-cgroups default",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", testutil.AlpineImage, "mkdir", "/sys/fs/cgroup/pids/foo")
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
	}
	testCase.Run(t)
}

// TestIssue3781 tests https://github.com/containerd/nerdctl/issues/3781
func TestIssue3781(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(
		require.Not(nerdtest.Docker),
		nerdtest.Info(func(info dockercompat.Info) error {
			if info.CgroupDriver == "none" || info.CgroupDriver == "" {
				return fmt.Errorf("test requires cgroup driver")
			}
			return nil
		}),
	)
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.AlpineImage, "sleep", "infinity")
		helpers.Ensure("update", "--cpuset-cpus", "0-1", data.Identifier())
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", "--format", "{{.Id}}", data.Identifier())
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			Output: func(stdout string, t tig.T) {
				cid := strings.TrimSpace(stdout)
				addr := defaults.DefaultAddress
				if rootlessutil.IsRootless() {
					stateDir, err := rootlessutil.RootlessKitStateDir()
					assert.NilError(t, err)
					childPid, err := rootlessutil.RootlessKitChildPid(stateDir)
					assert.NilError(t, err)
					addr = filepath.Join("/proc", fmt.Sprintf("%d", childPid), "root", defaults.DefaultAddress)
				}
				client, err := containerd.New(addr, containerd.WithDefaultNamespace(testutil.Namespace))
				assert.NilError(t, err)
				defer client.Close()
				ctx := context.Background()
				cntr, err := client.LoadContainer(ctx, cid)
				assert.NilError(t, err)
				spec, err := cntr.Spec(ctx)
				assert.NilError(t, err)
				assert.Assert(t, spec.Linux.Resources.Pids == nil)
			},
		}
	}
	testCase.Run(t)
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
			Expected: test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, t tig.T) {
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
	testCase := nerdtest.Setup()
	testCase.Require = require.All(
		// Docker lacks --cgroup-conf
		require.Not(nerdtest.Docker),
		nerdtest.CGroupV2,
		nerdtest.Info(func(info dockercompat.Info) error {
			if info.CgroupDriver == "none" || info.CgroupDriver == "" {
				return fmt.Errorf("test requires cgroup driver")
			}
			if !info.MemoryLimit {
				return fmt.Errorf("test requires MemoryLimit")
			}
			return nil
		}),
	)
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "--rm", "--cgroup-conf", "memory.high=33554432", "-w", "/sys/fs/cgroup", testutil.AlpineImage,
			"cat", "memory.high")
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			Output: expect.Equals("33554432\n"),
		}
	}
	testCase.Run(t)
}

func TestRunCgroupParent(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Info(func(info dockercompat.Info) error {
		if info.CgroupDriver == "none" || info.CgroupDriver == "" {
			return fmt.Errorf("test requires cgroup driver")
		}
		return nil
	})
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		cgroupDriver := strings.TrimSpace(helpers.Capture("info", "--format", "{{.CgroupDriver}}"))
		parent := "/foobarbaz"
		if cgroupDriver == "systemd" {
			// Path separators aren't allowed in systemd path. runc
			// explicitly checks for this.
			// https://github.com/opencontainers/runc/blob/016a0d29d1750180b2a619fc70d6fe0d80111be0/libcontainer/cgroups/systemd/common.go#L65-L68
			parent = "foobarbaz.slice"
		}
		data.Labels().Set("parent", parent)
		data.Labels().Set("cgroupDriver", cgroupDriver)
		// cgroup2 without host cgroup ns will just output 0::/ which doesn't help much to verify
		// we got our expected path. This approach should work for both cgroup1 and 2, there will
		// just be many more entries for cgroup1 as there'll be an entry per controller.
		helpers.Ensure("run", "-d", "--name", data.Identifier(),
			"--cgroupns=host", "--cgroup-parent", parent,
			testutil.AlpineImage, "sleep", "infinity")
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}
	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("exec", data.Identifier(), "cat", "/proc/self/cgroup")
	}
	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		parent := data.Labels().Get("parent")
		cgroupDriver := data.Labels().Get("cgroupDriver")
		id := strings.TrimSpace(helpers.Capture("inspect", "--format", "{{.Id}}", data.Identifier()))
		expected := filepath.Join(parent, id)
		if cgroupDriver == "systemd" {
			expected = filepath.Join(parent, fmt.Sprintf("nerdctl-%s", id))
			if nerdtest.IsDocker() {
				expected = filepath.Join(parent, fmt.Sprintf("docker-%s", id))
			}
		}
		return &test.Expected{
			Output: expect.Contains(expected),
		}
	}
	testCase.Run(t)
}

func TestRunBlkioWeightCgroupV2(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(
		nerdtest.CGroupV2,
		nerdtest.Info(func(info dockercompat.Info) error {
			if info.CgroupDriver == "none" || info.CgroupDriver == "" {
				return fmt.Errorf("test requires cgroup driver")
			}
			return nil
		}),
	)
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		if _, err := os.Stat("/sys/module/bfq"); err != nil {
			helpers.T().Skip(fmt.Sprintf("test requires \"bfq\" module to be loaded: %v", err))
		}
		// when bfq io scheduler is used, the io.weight knob is exposed as io.bfq.weight
		helpers.Ensure("run", "--name", data.Identifier(), "--blkio-weight", "300", "-w", "/sys/fs/cgroup", testutil.AlpineImage, "sleep", nerdtest.Infinity)
		data.Labels().Set("container", data.Identifier())
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}
	testCase.SubTests = []*test.Case{
		{
			Description: "initial weight",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Labels().Get("container"), "cat", "io.bfq.weight")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.Equals("default 300\n"),
				}
			},
		},
		{
			Description: "update weight",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("update", data.Labels().Get("container"), "--blkio-weight", "400")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Labels().Get("container"), "cat", "io.bfq.weight")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.Equals("default 400\n"),
				}
			},
		},
	}
	testCase.Run(t)
}

func TestRunBlkioSettingCgroupV2(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Rootful

	// See https://github.com/containerd/nerdctl/issues/4185
	// It is unclear if this is truly a kernel version problem, a runc issue, or a distro (EL9) issue.
	// For now, disable the test unless on a recent kernel.
	testutil.RequireKernelVersion(t, ">= 6.0.0-0")

	const (
		weight       = "150"
		deviceWeight = "100"
		readBps      = "1048576"
		readIops     = "1000"
		writeBps     = "2097152"
		writeIops    = "2000"
	)
	var lo *loopback.Loopback
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		var err error
		lo, err = loopback.New(4096)
		assert.NilError(t, err)
		t.Logf("loopback device: %+v", lo)
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if lo != nil {
			_ = lo.Close()
		}
	}
	testCase.SubTests = []*test.Case{
		{
			Description: "blkio-weight",
			Require:     nerdtest.CGroupV2,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "-d", "--name", data.Identifier(),
					"--blkio-weight", weight,
					testutil.AlpineImage, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(
						func(stdout string, t tig.T) {
							assert.Assert(t, strings.Contains(helpers.Capture("inspect", "--format", "{{.HostConfig.BlkioWeight}}", data.Identifier()), weight))
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
					"--blkio-weight-device", fmt.Sprintf("%s:%s", lo.Device, deviceWeight),
					testutil.AlpineImage, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(
						func(stdout string, t tig.T) {
							inspectOut := helpers.Capture("inspect", "--format", "{{range .HostConfig.BlkioWeightDevice}}{{.Weight}}{{end}}", data.Identifier())
							assert.Assert(t, strings.Contains(inspectOut, deviceWeight))
						},
						func(stdout string, t tig.T) {
							inspectOut := helpers.Capture("inspect", "--format", "{{range .HostConfig.BlkioWeightDevice}}{{.Path}}{{end}}", data.Identifier())
							assert.Assert(t, strings.Contains(inspectOut, lo.Device))
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
					"--device-read-bps", fmt.Sprintf("%s:%s", lo.Device, readBps),
					testutil.AlpineImage, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(
						func(stdout string, t tig.T) {
							inspectOut := helpers.Capture("inspect", "--format", "{{range .HostConfig.BlkioDeviceReadBps}}{{.Rate}}{{end}}", data.Identifier())
							assert.Assert(t, strings.Contains(inspectOut, readBps))
						},
						func(stdout string, t tig.T) {
							inspectOut := helpers.Capture("inspect", "--format", "{{range .HostConfig.BlkioDeviceReadBps}}{{.Path}}{{end}}", data.Identifier())
							assert.Assert(t, strings.Contains(inspectOut, lo.Device))
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
					"--device-write-bps", fmt.Sprintf("%s:%s", lo.Device, writeBps),
					testutil.AlpineImage, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(
						func(stdout string, t tig.T) {
							inspectOut := helpers.Capture("inspect", "--format", "{{range .HostConfig.BlkioDeviceWriteBps}}{{.Rate}}{{end}}", data.Identifier())
							assert.Assert(t, strings.Contains(inspectOut, writeBps))
						},
						func(stdout string, t tig.T) {
							inspectOut := helpers.Capture("inspect", "--format", "{{range .HostConfig.BlkioDeviceWriteBps}}{{.Path}}{{end}}", data.Identifier())
							assert.Assert(t, strings.Contains(inspectOut, lo.Device))
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
					"--device-read-iops", fmt.Sprintf("%s:%s", lo.Device, readIops),
					testutil.AlpineImage, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(
						func(stdout string, t tig.T) {
							inspectOut := helpers.Capture("inspect", "--format", "{{range .HostConfig.BlkioDeviceReadIOps}}{{.Rate}}{{end}}", data.Identifier())
							assert.Assert(t, strings.Contains(inspectOut, readIops))
						},
						func(stdout string, t tig.T) {
							inspectOut := helpers.Capture("inspect", "--format", "{{range .HostConfig.BlkioDeviceReadIOps}}{{.Path}}{{end}}", data.Identifier())
							assert.Assert(t, strings.Contains(inspectOut, lo.Device))
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
					"--device-write-iops", fmt.Sprintf("%s:%s", lo.Device, writeIops),
					testutil.AlpineImage, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(
						func(stdout string, t tig.T) {
							inspectOut := helpers.Capture("inspect", "--format", "{{range .HostConfig.BlkioDeviceWriteIOps}}{{.Rate}}{{end}}", data.Identifier())
							assert.Assert(t, strings.Contains(inspectOut, writeIops))
						},
						func(stdout string, t tig.T) {
							inspectOut := helpers.Capture("inspect", "--format", "{{range .HostConfig.BlkioDeviceWriteIOps}}{{.Path}}{{end}}", data.Identifier())
							assert.Assert(t, strings.Contains(inspectOut, lo.Device))
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
					func(stdout string, t tig.T) {
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

func TestRunCPUSharesCgroupV2(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: require.All(
			nerdtest.CGroupV2,
			nerdtest.Info(
				func(info dockercompat.Info) error {
					if !info.CPUShares {
						return fmt.Errorf("test requires CPUShares")
					}
					return nil
				},
			),
		),
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--rm", "--cpu-shares", "2000",
				testutil.AlpineImage, "cat", "/sys/fs/cgroup/cpu.weight")
		},
		// The value was historically 77, but with runc v1.4.0-rc.1 it became 170.
		// https://github.com/opencontainers/runc/issues/4896#issuecomment-3301825811
		Expected: test.Expects(0, nil, expect.Match(regexp.MustCompile("^(77|170)\n$"))),
	}

	testCase.Run(t)
}
