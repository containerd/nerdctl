//go:build unix

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

	"github.com/containerd/nerdctl/v2/pkg/infoutil"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestTop(t *testing.T) {
	//more details https://github.com/containerd/nerdctl/pull/223#issuecomment-851395178
	if runtime.GOOS == "linux" {
		if rootlessutil.IsRootless() && infoutil.CgroupsVersion() == "1" {
			t.Skip("test skipped for rootless containers on cgroup v1")
		}
	}

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", "inf")
		data.Set("cID", data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "with o pid,user,cmd",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("top", data.Get("cID"), "-o", "pid,user,cmd")
			},

			Expected: test.Expects(0, nil, nil),
		},
		{
			Description: "simple",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("top", data.Get("cID"))
			},

			Expected: test.Expects(0, nil, nil),
		},
	}

	testCase.Run(t)
}
