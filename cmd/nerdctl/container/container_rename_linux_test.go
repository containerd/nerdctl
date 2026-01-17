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
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestRename(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		testContainerName := data.Identifier()
		data.Labels().Set("containerName", testContainerName)
		helpers.Ensure("run", "-d", "--name", testContainerName, testutil.CommonImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, testContainerName)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		testContainerName := data.Labels().Get("containerName")
		helpers.Anyhow("rm", "-f", testContainerName)
		helpers.Anyhow("rm", "-f", testContainerName+"_new")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "`rename` should work",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				testContainerName := data.Labels().Get("containerName")
				return helpers.Command("rename", testContainerName, testContainerName+"_new")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "`rename` should have updated container name",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "-a")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				testContainerName := data.Labels().Get("containerName")
				return test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(testContainerName+"_new"))(data, helpers)
			},
		},
		{
			Description: "`rename` should fail to rename not existing container",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				testContainerName := data.Labels().Get("containerName")
				return helpers.Command("rename", testContainerName, testContainerName+"_new")
			},
			Expected: test.Expects(1, nil, nil),
		},
		{
			Description: "`rename` should fail to rename to existing name",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				testContainerName := data.Labels().Get("containerName")
				return helpers.Command("rename", testContainerName+"_new", testContainerName+"_new")
			},
			Expected: test.Expects(1, nil, nil),
		},
	}

	testCase.Run(t)
}

func TestRenameUpdateHosts(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		testContainerName := data.Identifier()
		data.Labels().Set("containerName", testContainerName)

		helpers.Ensure("run", "-d", "--name", testContainerName, testutil.CommonImage, "sleep", nerdtest.Infinity)
		helpers.Ensure("run", "-d", "--name", testContainerName+"_1", testutil.CommonImage, "sleep", nerdtest.Infinity)

		nerdtest.EnsureContainerStarted(helpers, testContainerName)
		nerdtest.EnsureContainerStarted(helpers, testContainerName+"_1")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		testContainerName := data.Labels().Get("containerName")
		helpers.Anyhow("rm", "-f", testContainerName)
		helpers.Anyhow("rm", "-f", testContainerName+"_1")
		helpers.Anyhow("rm", "-f", testContainerName+"_new")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "check '/etc/hosts' for sibling container",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				testContainerName := data.Labels().Get("containerName")
				return helpers.Command("exec", testContainerName, "cat", "/etc/hosts")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				testContainerName := data.Labels().Get("containerName")
				return test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(testContainerName+"_1"))(data, helpers)
			},
		},
		{
			Description: "rename container",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				testContainerName := data.Labels().Get("containerName")
				return helpers.Command("rename", testContainerName, testContainerName+"_new")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "check '/etc/hosts' for renamed container",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				testContainerName := data.Labels().Get("containerName")
				return helpers.Command("exec", testContainerName+"_new", "cat", "/etc/hosts")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				testContainerName := data.Labels().Get("containerName")
				return test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(testContainerName+"_new"))(data, helpers)
			},
		},
		{
			Description: "check sibling's '/etc/hosts' for renamed container",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				testContainerName := data.Labels().Get("containerName")
				return helpers.Command("exec", testContainerName+"_1", "cat", "/etc/hosts")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				testContainerName := data.Labels().Get("containerName")
				return test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(testContainerName+"_new"))(data, helpers)
			},
		},
	}

	testCase.Run(t)
}
