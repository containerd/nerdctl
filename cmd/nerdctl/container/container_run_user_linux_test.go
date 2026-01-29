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

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestRunUserGID(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.SubTests = []*test.Case{
		{
			Description: "Test container run as default user (root) and verify root belongs to standard system groups",
			Command:     test.Command("run", "--rm", testutil.AlpineImage, "id", "-nG"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("root bin daemon sys adm disk wheel floppy dialout tape video")),
		},
		{
			Description: "Test container run with numeric UID (1000) and verify it resolves to root group inside the container",
			Command:     test.Command("run", "--rm", "--user", "1000", testutil.AlpineImage, "id", "-nG"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("root")),
		},
		{
			Description: "Test container run as user (guest) and verify group membership is resolved correctly",
			Command:     test.Command("run", "--rm", "--user", "guest", testutil.AlpineImage, "id", "-nG"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("users")),
		},
		{
			Description: "Test container run with well-known user 'nobody' and verify it belongs to the 'nobody' group",
			Command:     test.Command("run", "--rm", "--user", "nobody", testutil.AlpineImage, "id", "-nG"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("nobody")),
		},
	}
	testCase.Run(t)
}

func TestRunUmask(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)
	testCase.Command = test.Command("run", "--rm", "--umask", "0200", testutil.AlpineImage, "sh", "-c", "umask")
	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("0200"))
	testCase.Run(t)
}

func TestRunAddGroup(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.SubTests = []*test.Case{
		{
			Description: "Test container run as default root user and its inherited system groups",
			Command:     test.Command("run", "--rm", testutil.AlpineImage, "id", "-nG"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("root bin daemon sys adm disk wheel floppy dialout tape video\n")),
		},
		{
			Description: "Test container run as numeric UID only and its fallback to root group",
			Command:     test.Command("run", "--rm", "--user", "1000", testutil.AlpineImage, "id", "-nG"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("root\n")),
		},
		{
			Description: "Test container run as numeric UID with extra group addition",
			Command:     test.Command("run", "--rm", "--user", "1000", "--group-add", "nogroup", testutil.AlpineImage, "id", "-nG"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("root nogroup\n")),
		},
		{
			Description: "Test container run as UID:GID pair with extra group addition",
			Command:     test.Command("run", "--rm", "--user", "1000:wheel", "--group-add", "nogroup", testutil.AlpineImage, "id", "-nG"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("wheel nogroup\n")),
		},
		{
			Description: "Test container run as root with extra group addition and system group persistence",
			Command:     test.Command("run", "--rm", "--user", "root", "--group-add", "nogroup", testutil.AlpineImage, "id", "-nG"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("root bin daemon sys adm disk wheel floppy dialout tape video nogroup\n")),
		},
		{
			Description: "Test container run as root:group override and its effect on supplementary groups",
			Command:     test.Command("run", "--rm", "--user", "root:nogroup", "--group-add", "nogroup", testutil.AlpineImage, "id", "-nG"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("nogroup\n")),
		},
		{
			Description: "Test container run as named non-root user with multiple group additions",
			Command:     test.Command("run", "--rm", "--user", "guest", "--group-add", "root", "--group-add", "nogroup", testutil.AlpineImage, "id", "-nG"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("users root nogroup\n")),
		},
		{
			Description: "Test container run as named user:group with numeric GID resolution",
			Command:     test.Command("run", "--rm", "--user", "guest:nogroup", "--group-add", "0", testutil.AlpineImage, "id", "-nG"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("nogroup root\n")),
		},
	}
	testCase.Run(t)
}

// TestRunAddGroup_CVE_2023_25173 tests https://github.com/advisories/GHSA-hmfx-3pcx-653p
//
// Equates to https://github.com/containerd/containerd/commit/286a01f350a2298b4fdd7e2a0b31c04db3937ea8
func TestRunAddGroup_CVE_2023_25173(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("pull", "--quiet", testutil.BusyboxImage)
	}
	testCase.SubTests = []*test.Case{
		{
			Description: "Test container run as default root user",
			Command:     test.Command("run", "--rm", testutil.BusyboxImage, "id"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("groups=0(root),10(wheel)\n")),
		},
		{
			Description: "Test container run as root with additional groups",
			Command:     test.Command("run", "--rm", "--group-add", "1", "--group-add", "1234", testutil.BusyboxImage, "id"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("groups=0(root),1(daemon),10(wheel),1234\n")),
		},
		{
			Description: "Test container run as custom UID with inherited root group",
			Command:     test.Command("run", "--rm", "--user", "1234", testutil.BusyboxImage, "id"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("groups=0(root)\n")),
		},
		{
			Description: "Test container run as custom UID and GID pair",
			Command:     test.Command("run", "--rm", "--user", "1234:1234", testutil.BusyboxImage, "id"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("groups=1234\n")),
		},
		{
			Description: "Test container run as custom UID with explicit group add",
			Command:     test.Command("run", "--rm", "--user", "1234", "--group-add", "1234", testutil.BusyboxImage, "id"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("groups=0(root),1234\n")),
		},
		{
			Description: "Test container run as named non-root user (daemon)",
			Command:     test.Command("run", "--rm", "--user", "daemon", testutil.BusyboxImage, "id"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("groups=1(daemon)\n")),
		},
		{
			Description: "Test container run as named user with extra groups",
			Command:     test.Command("run", "--rm", "--user", "daemon", "--group-add", "1234", testutil.BusyboxImage, "id"),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("groups=1(daemon),1234\n")),
		},
	}
	testCase.Run(t)
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
						Output: func(stdout string, t tig.T) {
							actualHostUID, err := getContainerHostUID(helpers, data.Identifier())
							if err != nil {
								t.Log(fmt.Sprintf("Failed to get container host UID: %v", err))
								t.FailNow()
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
						Output: func(stdout string, t tig.T) {
							actualHostUID, err := getContainerHostUID(helpers, data.Identifier())
							if err != nil {
								t.Log(fmt.Sprintf("Failed to get container host UID: %v", err))
								t.FailNow()
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
						Output: func(stdout string, t tig.T) {
							actualHostUID, err := getContainerHostUID(helpers, data.Identifier())
							if err != nil {
								t.Log(fmt.Sprintf("Failed to get container host UID: %v", err))
								t.FailNow()
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
						Output: func(stdout string, t tig.T) {
							actualHostUID, err := getContainerHostUID(helpers, data.Identifier())
							if err != nil {
								t.Log(fmt.Sprintf("Failed to get container host UID: %v", err))
								t.FailNow()
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
						Output: func(stdout string, t tig.T) {
							actualHostUID, err := getContainerHostUID(helpers, data.Identifier())
							if err != nil {
								t.Log(fmt.Sprintf("Failed to get container host UID: %v", err))
								t.FailNow()
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
