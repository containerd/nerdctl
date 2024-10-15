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

	"github.com/containerd/nerdctl/v2/pkg/infoutil"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestStats(t *testing.T) {
	// this comment is for `nerdctl ps` but it also valid for `nerdctl stats` :
	// https://github.com/containerd/nerdctl/pull/223#issuecomment-851395178
	if rootlessutil.IsRootless() && infoutil.CgroupsVersion() == "1" {
		t.Skip("test skipped for rootless containers on cgroup v1")
	}

	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier()[:12])
		helpers.Anyhow("rm", "-f", data.Identifier()[:12]+"-exited")
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "--name", data.Identifier()[:12], testutil.AlpineImage, "sleep", "inf")
		helpers.Ensure("run", "--name", data.Identifier()[:12]+"-exited", testutil.AlpineImage, "echo", "'exited'")
		data.Set("id", data.Identifier()[:12])
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "stats",
			Command:     test.Command("stats", "--no-stream"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.Contains(data.Get("id")),
				}
			},
		},
		{
			Description: "container stats",
			Command:     test.Command("container", "stats", "--no-stream"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.Contains(data.Get("id")),
				}
			},
		},
		{
			Description: "stats ID",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("stats", "--no-stream", data.Get("id"))
			},
			Expected: test.Expects(0, nil, nil),
		},
		{
			Description: "container stats  ID",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("container", "stats", "--no-stream", data.Get("id"))
			},
			Expected: test.Expects(0, nil, nil),
		},
	}

	testCase.Run(t)
}
