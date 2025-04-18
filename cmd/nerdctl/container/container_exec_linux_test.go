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
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestExecWithUser(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	testContainer := testutil.Identifier(t)

	defer base.Cmd("rm", "-f", testContainer).Run()
	base.Cmd("run", "-d", "--name", testContainer, testutil.CommonImage, "sleep", nerdtest.Infinity).AssertOK()
	base.EnsureContainerStarted(testContainer)

	testCases := map[string]string{
		"":             "uid=0(root) gid=0(root)",
		"1000":         "uid=1000 gid=0(root)",
		"1000:users":   "uid=1000 gid=100(users)",
		"guest":        "uid=405(guest) gid=100(users)",
		"nobody":       "uid=65534(nobody) gid=65534(nobody)",
		"nobody:users": "uid=65534(nobody) gid=100(users)",
	}

	for userStr, expected := range testCases {
		cmd := []string{"exec"}
		if userStr != "" {
			cmd = append(cmd, "--user", userStr)
		}
		cmd = append(cmd, testContainer, "id")
		base.Cmd(cmd...).AssertOutContains(expected)
	}
}

func TestExecTTY(t *testing.T) {
	const sttyPartialOutput = "speed 38400 baud"

	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("rm", "-f", data.Identifier())
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
		data.Labels().Set("container_name", data.Identifier())
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "stty with -it",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("exec", "-it", data.Labels().Get("container_name"), "stty")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(0, nil, expect.Contains(sttyPartialOutput)),
		},
		{
			Description: "stty with -t",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("exec", "-t", data.Labels().Get("container_name"), "stty")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(0, nil, expect.Contains(sttyPartialOutput)),
		},
		{
			Description: "stty with -i",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("exec", "-i", data.Labels().Get("container_name"), "stty")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "stty without params",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("exec", data.Labels().Get("container_name"), "stty")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
	}

	testCase.Run(t)
}
