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
	"runtime"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestStats(t *testing.T) {
	testCase := nerdtest.Setup()

	// FIXME: does not seem to work on windows
	testCase.Require = require.Not(require.Windows)

	if runtime.GOOS == "linux" {
		// this comment is for `nerdctl ps` but it also valid for `nerdctl stats` :
		// https://github.com/containerd/nerdctl/pull/223#issuecomment-851395178
		testCase.Require = require.All(
			testCase.Require,
			nerdtest.CgroupsAccessible,
		)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("container"))
		helpers.Anyhow("rm", "-f", data.Identifier("memlimited"))
		helpers.Anyhow("rm", "-f", data.Identifier("exited"))
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier("container"), testutil.CommonImage, "sleep", nerdtest.Infinity)
		helpers.Ensure("run", "-d", "--name", data.Identifier("memlimited"), "--memory", "1g", testutil.CommonImage, "sleep", nerdtest.Infinity)
		helpers.Ensure("run", "--name", data.Identifier("exited"), testutil.CommonImage, "echo", "'exited'")
		data.Labels().Set("id", data.Identifier("container"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "stats",
			Command:     test.Command("stats", "--no-stream", "--no-trunc"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.Contains(data.Labels().Get("id")),
				}
			},
		},
		{
			Description: "container stats",
			Command:     test.Command("container", "stats", "--no-stream", "--no-trunc"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.Contains(data.Labels().Get("id")),
				}
			},
		},
		{
			Description: "stats ID",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("stats", "--no-stream", data.Labels().Get("id"))
			},
			Expected: test.Expects(0, nil, nil),
		},
		{
			Description: "container stats ID",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("container", "stats", "--no-stream", data.Labels().Get("id"))
			},
			Expected: test.Expects(0, nil, nil),
		},
		{
			Description: "no mem limit set",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("stats", "--no-stream")
			},
			// https://github.com/containerd/nerdctl/issues/1240
			// nerdctl used to print UINT64_MAX as the memory limit, so, ensure it does no more
			Expected: test.Expects(0, nil, expect.DoesNotContain("16EiB")),
		},
		{
			Description: "mem limit set",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("stats", "--no-stream")
			},
			Expected: test.Expects(0, nil, expect.Contains("1GiB")),
		},
	}

	testCase.Run(t)
}
