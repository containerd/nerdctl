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
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/apparmorutil"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/testutil"

	"gotest.tools/v3/assert"
)

func getCapEff(base *testutil.Base, args ...string) uint64 {
	fullArgs := []string{"run", "--rm"}
	fullArgs = append(fullArgs, args...)
	fullArgs = append(fullArgs,
		testutil.AlpineImage,
		"sh",
		"-euc",
		"grep -w ^CapEff: /proc/self/status | sed -e \"s/^CapEff:[[:space:]]*//g\"",
	)
	cmd := base.Cmd(fullArgs...)
	res := cmd.Run()
	assert.NilError(base.T, res.Error)
	s := strings.TrimSpace(res.Stdout())
	ui64, err := strconv.ParseUint(s, 16, 64)
	assert.NilError(base.T, err)
	return ui64
}

const (
	CapNetRaw  = 13
	CapIPCLock = 14
)

func TestRunCap(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)

	// allCaps varies depending on the target version and the kernel version.
	allCaps := getCapEff(base, "--privileged")

	// https://github.com/containerd/containerd/blob/9a9bd097564b0973bfdb0b39bf8262aa1b7da6aa/oci/spec.go#L93
	defaultCaps := uint64(0xa80425fb)

	t.Logf("allCaps=%016x", allCaps)

	type testCase struct {
		args   []string
		capEff uint64
	}
	testCases := []testCase{
		{
			capEff: allCaps & defaultCaps,
		},
		{
			args:   []string{"--cap-add=all"},
			capEff: allCaps,
		},
		{
			args:   []string{"--cap-add=ipc_lock"},
			capEff: (allCaps & defaultCaps) | (1 << CapIPCLock),
		},
		{
			args:   []string{"--cap-add=all", "--cap-drop=net_raw"},
			capEff: allCaps ^ (1 << CapNetRaw),
		},
		{
			args:   []string{"--cap-drop=all", "--cap-add=net_raw"},
			capEff: 1 << CapNetRaw,
		},
		{
			args:   []string{"--cap-drop=all", "--cap-add=NET_RAW"},
			capEff: 1 << CapNetRaw,
		},
		{
			args:   []string{"--cap-drop=all", "--cap-add=cap_net_raw"},
			capEff: 1 << CapNetRaw,
		},
		{
			args:   []string{"--cap-drop=all", "--cap-add=CAP_NET_RAW"},
			capEff: 1 << CapNetRaw,
		},
	}
	for _, tc := range testCases {
		tc := tc // IMPORTANT
		name := "default"
		if len(tc.args) > 0 {
			name = strings.Join(tc.args, "_")
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := getCapEff(base, tc.args...)
			assert.Equal(t, tc.capEff, got)
		})
	}
}

func TestRunSecurityOptSeccomp(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	type testCase struct {
		args    []string
		seccomp int
	}
	testCases := []testCase{
		{
			seccomp: 2,
		},
		{
			args:    []string{"--security-opt", "seccomp=unconfined"},
			seccomp: 0,
		},
		{
			args:    []string{"--privileged"},
			seccomp: 0,
		},
	}
	for _, tc := range testCases {
		tc := tc // IMPORTANT
		name := "default"
		if len(tc.args) > 0 {
			name = strings.Join(tc.args, "_")
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			args := []string{"run", "--rm"}
			args = append(args, tc.args...)
			// NOTE: busybox grep does not support -oP \K
			args = append(args, testutil.AlpineImage, "grep", "-Eo", `^Seccomp:\s*([0-9]+)`, "/proc/1/status")
			cmd := base.Cmd(args...)
			f := func(expectedSeccomp int) func(string) error {
				return func(stdout string) error {
					s := strings.TrimPrefix(stdout, "Seccomp:")
					s = strings.TrimSpace(s)
					i, err := strconv.Atoi(s)
					if err != nil {
						return fmt.Errorf("failed to parse line %q: %w", stdout, err)
					}
					if i != expectedSeccomp {
						return fmt.Errorf("expected Seccomp to be %d, got %d", expectedSeccomp, i)
					}
					return nil
				}
			}
			cmd.AssertOutWithFunc(f(tc.seccomp))
		})
	}
}

func TestRunApparmor(t *testing.T) {
	base := testutil.NewBase(t)
	defaultProfile := fmt.Sprintf("%s-default", base.Target)
	if !apparmorutil.CanLoadNewProfile() && !apparmorutil.CanApplySpecificExistingProfile(defaultProfile) {
		t.Skipf("needs to be able to apply %q profile", defaultProfile)
	}
	attrCurrentPath := "/proc/self/attr/apparmor/current"
	if _, err := os.Stat(attrCurrentPath); err != nil {
		attrCurrentPath = "/proc/self/attr/current"
	}
	attrCurrentEnforceExpected := fmt.Sprintf("%s (enforce)\n", defaultProfile)
	base.Cmd("run", "--rm", testutil.AlpineImage, "cat", attrCurrentPath).AssertOutExactly(attrCurrentEnforceExpected)
	base.Cmd("run", "--rm", "--security-opt", "apparmor="+defaultProfile, testutil.AlpineImage, "cat", attrCurrentPath).AssertOutExactly(attrCurrentEnforceExpected)
	base.Cmd("run", "--rm", "--security-opt", "apparmor=unconfined", testutil.AlpineImage, "cat", attrCurrentPath).AssertOutExactly("unconfined\n")
	base.Cmd("run", "--rm", "--privileged", testutil.AlpineImage, "cat", attrCurrentPath).AssertOutExactly("unconfined\n")
}

// TestRunSeccompCapSysPtrace tests https://github.com/containerd/nerdctl/issues/976
func TestRunSeccompCapSysPtrace(t *testing.T) {
	base := testutil.NewBase(t)
	base.Cmd("run", "--rm", "--cap-add", "sys_ptrace", testutil.AlpineImage, "sh", "-euxc", "apk add -q strace && strace true").AssertOK()
	// Docker/Moby 's seccomp profile allows ptrace(2) by default, but containerd does not (yet): https://github.com/containerd/containerd/issues/6802
}

func TestRunPrivileged(t *testing.T) {
	// docker does not support --privileged-without-host-devices
	testutil.DockerIncompatible(t)

	if rootlessutil.IsRootless() {
		t.Skip("test skipped for rootless privileged containers")
	}

	base := testutil.NewBase(t)

	devPath := "/dev/dummy-zero"

	// a dummy zero device: mknod /dev/dummy-zero c 1 5
	helperCmd := exec.Command("mknod", []string{devPath, "c", "1", "5"}...)
	if out, err := helperCmd.CombinedOutput(); err != nil {
		err = fmt.Errorf("cannot create %q: %q: %w", devPath, string(out), err)
		t.Fatal(err)
	}

	// ensure the file will be removed in case of failed in the test
	defer func() {
		exec.Command("rm", devPath).Run()
	}()

	// get device with host devices
	base.Cmd("run", "--rm", "--privileged", testutil.AlpineImage, "ls", devPath).AssertOutExactly(devPath + "\n")

	// get device without host devices
	res := base.Cmd("run", "--rm", "--privileged", "--security-opt", "privileged-without-host-devices", testutil.AlpineImage, "ls", devPath).Run()

	// normally for not a exists file, the `ls` will return `1``.
	assert.Check(t, res.ExitCode != 0, res.Combined())

	// something like `ls: /dev/dummy-zero: No such file or directory`
	assert.Check(t, strings.Contains(res.Combined(), "No such file or directory"))
}
