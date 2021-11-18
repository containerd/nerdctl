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
	"strconv"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/apparmorutil"
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
	CAP_NET_RAW  = 13
	CAP_IPC_LOCK = 14
)

func TestRunCap(t *testing.T) {
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
			capEff: (allCaps & defaultCaps) | (1 << CAP_IPC_LOCK),
		},
		{
			args:   []string{"--cap-add=all", "--cap-drop=net_raw"},
			capEff: allCaps ^ (1 << CAP_NET_RAW),
		},
		{
			args:   []string{"--cap-drop=all", "--cap-add=net_raw"},
			capEff: 1 << CAP_NET_RAW,
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
