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
	"errors"
	"runtime"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestTop(t *testing.T) {
	testCase := nerdtest.Setup()

	//more details https://github.com/containerd/nerdctl/pull/223#issuecomment-851395178
	if runtime.GOOS == "linux" {
		testCase.Require = nerdtest.CgroupsAccessible
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// FIXME: busybox 1.36 on windows still appears to not support sleep inf. Unclear why.
		helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
		data.Labels().Set("cID", data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "with o pid,user,cmd",
			// Docker does not support top -o
			Require: require.Not(nerdtest.Docker),
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("top", data.Labels().Get("cID"), "-o", "pid,user,cmd")
			},

			Expected: test.Expects(0, nil, nil),
		},
		{
			Description: "simple",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("top", data.Labels().Get("cID"))
			},

			Expected: test.Expects(0, nil, nil),
		},
	}

	testCase.Run(t)
}

func TestTopStoppedContainer(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(require.Linux, nerdtest.CgroupsAccessible)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--network", "none", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
		helpers.Ensure("stop", "--time", "1", data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("top", data.Identifier())
	}

	testCase.Expected = test.Expects(1, []error{errors.New("is not running")}, nil)
	testCase.Run(t)
}

func TestTopPausedContainer(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.All(require.Linux, nerdtest.CgroupsAccessible)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--network", "none", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
		helpers.Ensure("pause", data.Identifier())
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("top", data.Identifier())
	}

	testCase.Expected = test.Expects(0, nil, expect.Contains("sleep"))
	testCase.Run(t)
}

func TestTopHyperVContainer(t *testing.T) {

	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.HyperV

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// FIXME: busybox 1.36 on windows still appears to not support sleep inf. Unclear why.
		helpers.Ensure("run", "--isolation", "hyperv", "-d", "--name", data.Identifier("container"), testutil.CommonImage, "sleep", nerdtest.Infinity)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("container"))
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("top", data.Identifier("container"))
	}

	testCase.Expected = test.Expects(0, nil, nil)

	testCase.Run(t)
}
