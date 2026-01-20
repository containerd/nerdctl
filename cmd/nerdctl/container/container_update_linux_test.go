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
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestUpdateContainer(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		containerName := testutil.Identifier(t)
		data.Labels().Set("containerName", containerName)
		helpers.Ensure("run", "-d", "--name", containerName, testutil.CommonImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, containerName)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		containerName := data.Labels().Get("containerName")
		helpers.Anyhow("rm", "-f", containerName)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "should fail on unsupported restart policy value",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				containerName := data.Labels().Get("containerName")
				return helpers.Command("update", "--memory", "999999999", "--restart", "123", containerName)
			},
			Expected: test.Expects(1, []error{errors.New("unsupported restart policy")}, nil),
		},
		{
			Description: "should not update memory in inspect",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				containerName := data.Labels().Get("containerName")
				return helpers.Command("inspect", "--mode=native", containerName)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.DoesNotContain(`"limit": 999999999,`)),
		},
	}

	testCase.Run(t)
}
