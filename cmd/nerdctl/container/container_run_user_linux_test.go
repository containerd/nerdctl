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
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestRunUserGID(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	testCases := map[string]string{
		"":       "root bin daemon sys adm disk wheel floppy dialout tape video",
		"1000":   "root",
		"guest":  "users",
		"nobody": "nobody",
	}
	for userStr, expected := range testCases {
		userStr := userStr
		expected := expected
		t.Run(userStr, func(t *testing.T) {
			t.Parallel()
			cmd := []string{"run", "--rm"}
			if userStr != "" {
				cmd = append(cmd, "--user", userStr)
			}
			cmd = append(cmd, testutil.AlpineImage, "id", "-nG")
			base.Cmd(cmd...).AssertOutContains(expected)
		})
	}
}

func TestRunUmask(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	testutil.DockerIncompatible(t)
	base.Cmd("run", "--rm", "--umask", "0200", testutil.AlpineImage, "sh", "-c", "umask").AssertOutContains("0200")
}

func TestRunAddGroup(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	testCases := []struct {
		user     string
		groups   []string
		expected string
	}{
		{
			user:     "",
			groups:   []string{},
			expected: "root bin daemon sys adm disk wheel floppy dialout tape video",
		},
		{
			user:     "1000",
			groups:   []string{},
			expected: "root",
		},
		{
			user:     "1000",
			groups:   []string{"nogroup"},
			expected: "root nogroup",
		},
		{
			user:     "1000:wheel",
			groups:   []string{"nogroup"},
			expected: "wheel nogroup",
		},
		{
			user:     "root",
			groups:   []string{"nogroup"},
			expected: "root bin daemon sys adm disk wheel floppy dialout tape video nogroup",
		},
		{
			user:     "root:nogroup",
			groups:   []string{"nogroup"},
			expected: "nogroup",
		},
		{
			user:     "guest",
			groups:   []string{"root", "nogroup"},
			expected: "users root nogroup",
		},
		{
			user:     "guest:nogroup",
			groups:   []string{"0"},
			expected: "nogroup root",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.user, func(t *testing.T) {
			t.Parallel()
			cmd := []string{"run", "--rm"}
			if testCase.user != "" {
				cmd = append(cmd, "--user", testCase.user)
			}
			for _, group := range testCase.groups {
				cmd = append(cmd, "--group-add", group)
			}
			cmd = append(cmd, testutil.AlpineImage, "id", "-nG")
			base.Cmd(cmd...).AssertOutExactly(testCase.expected + "\n")
		})
	}
}

// TestRunAddGroup_CVE_2023_25173 tests https://github.com/advisories/GHSA-hmfx-3pcx-653p
//
// Equates to https://github.com/containerd/containerd/commit/286a01f350a2298b4fdd7e2a0b31c04db3937ea8
func TestRunAddGroup_CVE_2023_25173(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	testCases := []struct {
		user     string
		groups   []string
		expected string
	}{
		{
			user:     "",
			groups:   nil,
			expected: "groups=0(root),10(wheel)",
		},
		{
			user:     "",
			groups:   []string{"1", "1234"},
			expected: "groups=0(root),1(daemon),10(wheel),1234",
		},
		{
			user:     "1234",
			groups:   nil,
			expected: "groups=0(root)",
		},
		{
			user:     "1234:1234",
			groups:   nil,
			expected: "groups=1234",
		},
		{
			user:     "1234",
			groups:   []string{"1234"},
			expected: "groups=0(root),1234",
		},
		{
			user:     "daemon",
			groups:   nil,
			expected: "groups=1(daemon)",
		},
		{
			user:     "daemon",
			groups:   []string{"1234"},
			expected: "groups=1(daemon),1234",
		},
	}

	base.Cmd("pull", "--quiet", testutil.BusyboxImage).AssertOK()
	for _, testCase := range testCases {
		cmd := []string{"run", "--rm"}
		if testCase.user != "" {
			cmd = append(cmd, "--user", testCase.user)
		}
		for _, group := range testCase.groups {
			cmd = append(cmd, "--group-add", group)
		}
		cmd = append(cmd, testutil.BusyboxImage, "id")
		base.Cmd(cmd...).AssertOutContains(testCase.expected + "\n")
	}
}

func TestUsernsMappingRunCmd(t *testing.T) {
	nerdtest.Setup()
	testCase := &test.Case{
		Require: require.All(
			nerdtest.AllowModifyUserns,
			nerdtest.RemapIDs,
			require.Not(nerdtest.Docker),
		),
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Labels().Set("validUserns", "nerdctltestuser")
			data.Labels().Set("expectedHostUID", "123456789")
			data.Labels().Set("validUid", "123456789")
			data.Labels().Set("net-container", "net-container")
			data.Labels().Set("invalidUserns", "invaliduser")
		},
		SubTests: []*test.Case{
			{
				Description: "Test container run with valid Userns format userns username",
				NoParallel:  true, // Changes system config so running in non parallel mode
				Setup: func(data test.Data, helpers test.Helpers) {
					err := appendUsernsConfig(data.Labels().Get("validUserns"), data.Labels().Get("expectedHostUID"), helpers)
					assert.NilError(t, err, "Failed to append Userns config")
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rm", "-f", data.Identifier())
					removeUsernsConfig(t, data.Labels().Get("validUserns"), helpers)
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--tty", "-d", "--userns-remap", data.Labels().Get("validUserns"), "--name", data.Identifier(), testutil.CommonImage, "sleep", "inf")
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						ExitCode: 0,
						Output: func(stdout string, t *testing.T) {
							actualHostUID, err := getContainerHostUID(helpers, data.Identifier())
							if err != nil {
								t.Fatalf("Failed to get container host UID: %v", err)
							}
							assert.Assert(t, actualHostUID == data.Labels().Get("expectedHostUID"))
						},
					}
				},
			},
			{
				Description: "Test container run with valid Userns  --userns uid",
				NoParallel:  true, // Changes system config so running in non parallel mode
				Setup: func(data test.Data, helpers test.Helpers) {
					err := appendUsernsConfig(data.Labels().Get("validUserns"), data.Labels().Get("expectedHostUID"), helpers)
					assert.NilError(t, err, "Failed to append Userns config")
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rm", "-f", data.Identifier())
					removeUsernsConfig(t, data.Labels().Get("validUserns"), helpers)
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--tty", "-d", "--userns-remap", data.Labels().Get("validUid"), "--name", data.Identifier(), testutil.CommonImage, "sleep", "inf")
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						ExitCode: 0,
						Output: func(stdout string, t *testing.T) {
							actualHostUID, err := getContainerHostUID(helpers, data.Identifier())
							if err != nil {
								t.Fatalf("Failed to get container host UID: %v", err)
							}
							assert.Assert(t, actualHostUID == data.Labels().Get("expectedHostUID"))
						},
					}
				},
			},
			{
				Description: "Test container run failure with valid Userns and privileged flag",
				NoParallel:  true, // Changes system config so running in non parallel mode
				Setup: func(data test.Data, helpers test.Helpers) {
					err := appendUsernsConfig(data.Labels().Get("validUserns"), data.Labels().Get("expectedHostUID"), helpers)
					assert.NilError(t, err, "Failed to append Userns config")
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					removeUsernsConfig(t, data.Labels().Get("validUserns"), helpers)
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--tty", "--privileged", "--userns-remap", data.Labels().Get("validUserns"), "--name", data.Identifier(), testutil.CommonImage, "sleep", "inf")
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						ExitCode: 1,
					}
				},
			},
			{
				Description: "Test container run with valid Userns format --userns <username>:<groupname>",
				NoParallel:  true, // Changes system config so running in non parallel mode
				Setup: func(data test.Data, helpers test.Helpers) {
					err := appendUsernsConfig(data.Labels().Get("validUserns"), data.Labels().Get("expectedHostUID"), helpers)
					assert.NilError(t, err, "Failed to append Userns config")
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rm", "-f", data.Identifier())
					removeUsernsConfig(t, data.Labels().Get("validUserns"), helpers)
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--tty", "-d", "--userns-remap", fmt.Sprintf("%s:%s", data.Labels().Get("validUserns"), data.Labels().Get("validUserns")), "--name", data.Identifier(), testutil.CommonImage, "sleep", "inf")
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						ExitCode: 0,
						Output: func(stdout string, t *testing.T) {
							actualHostUID, err := getContainerHostUID(helpers, data.Identifier())
							if err != nil {
								t.Fatalf("Failed to get container host UID: %v", err)
							}
							assert.Assert(t, actualHostUID == data.Labels().Get("expectedHostUID"))
						},
					}
				},
			},
			{
				Description: "Test container run with valid Userns  --userns uid:gid",
				NoParallel:  true, // Changes system config so running in non parallel mode
				Setup: func(data test.Data, helpers test.Helpers) {
					err := appendUsernsConfig(data.Labels().Get("validUserns"), data.Labels().Get("expectedHostUID"), helpers)
					assert.NilError(t, err, "Failed to append Userns config")
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rm", "-f", data.Identifier())
					removeUsernsConfig(t, data.Labels().Get("validUserns"), helpers)
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--tty", "-d", "--userns-remap", fmt.Sprintf("%s:%s", data.Labels().Get("validUid"), data.Labels().Get("validUid")), "--name", data.Identifier(), testutil.CommonImage, "sleep", "inf")
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						ExitCode: 0,
						Output: func(stdout string, t *testing.T) {
							actualHostUID, err := getContainerHostUID(helpers, data.Identifier())
							if err != nil {
								t.Fatalf("Failed to get container host UID: %v", err)
							}
							assert.Assert(t, actualHostUID == data.Labels().Get("expectedHostUID"))
						},
					}
				},
			},
			{
				Description: "Test container network share with valid Userns",
				NoParallel:  true, // Changes system config so running in non parallel mode
				Setup: func(data test.Data, helpers test.Helpers) {
					err := appendUsernsConfig(data.Labels().Get("validUserns"), data.Labels().Get("expectedHostUID"), helpers)
					assert.NilError(t, err, "Failed to append Userns config")
					helpers.Ensure("run", "--tty", "-d", "--userns-remap", data.Labels().Get("validUserns"), "--name", data.Labels().Get("net-container"), testutil.CommonImage, "sleep", "inf")
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rm", "-f", data.Identifier())
					helpers.Anyhow("rm", "-f", data.Labels().Get("net-container"))
					removeUsernsConfig(t, data.Labels().Get("validUserns"), helpers)
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--tty", "-d", "--userns-remap", data.Labels().Get("validUserns"), "--net", fmt.Sprintf("container:%s", data.Labels().Get("net-container")), "--name", data.Identifier(), testutil.CommonImage, "sleep", "inf")
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "Test container run with valid Userns with override --userns=host",
				NoParallel:  true, // Changes system config so running in non parallel mode
				Setup: func(data test.Data, helpers test.Helpers) {
					err := appendUsernsConfig(data.Labels().Get("validUserns"), data.Labels().Get("expectedHostUID"), helpers)
					assert.NilError(t, err, "Failed to append Userns config")
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rm", "-f", data.Identifier())
					removeUsernsConfig(t, data.Labels().Get("validUserns"), helpers)
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--tty", "-d", "--userns-remap", data.Labels().Get("validUserns"), "--userns", "host", "--name", data.Identifier(), testutil.CommonImage, "sleep", "inf")
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						ExitCode: 0,
						Output: func(stdout string, t *testing.T) {
							actualHostUID, err := getContainerHostUID(helpers, data.Identifier())
							if err != nil {
								t.Fatalf("Failed to get container host UID: %v", err)
							}
							assert.Assert(t, actualHostUID == "0")
						},
					}
				},
			},
			{
				Description: "Test container run with valid Userns with invalid overrid --userns=hostinvalid",
				NoParallel:  true, // Changes system config so running in non parallel mode
				Setup: func(data test.Data, helpers test.Helpers) {
					err := appendUsernsConfig(data.Labels().Get("validUserns"), data.Labels().Get("expectedHostUID"), helpers)
					assert.NilError(t, err, "Failed to append Userns config")
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rm", "-f", data.Identifier())
					removeUsernsConfig(t, data.Labels().Get("validUserns"), helpers)
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--tty", "-d", "--userns-remap", data.Labels().Get("validUserns"), "--userns", "hostinvalid", "--name", data.Identifier(), testutil.CommonImage, "sleep", "inf")
				},
				Expected: test.Expects(1, nil, nil),
			},
			{
				Description: "Test container run with invalid Userns",
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rm", "-f", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--tty", "-d", "--userns-remap", data.Labels().Get("invalidUserns"), "--name", data.Identifier(), testutil.CommonImage, "sleep", "inf")
				},
				Expected: test.Expects(1, nil, nil),
			},
		},
	}
	testCase.Run(t)
}
