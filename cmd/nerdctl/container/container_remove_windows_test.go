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

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestRemoveHyperVContainer(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.HyperV

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--isolation", "hyperv", "--name", testutil.Identifier(t), testutil.CommonImage, "sleep", nerdtest.Infinity)
		nerdtest.EnsureContainerStarted(helpers, testutil.Identifier(t))

		inspect := nerdtest.InspectContainer(helpers, testutil.Identifier(t))
		//check with HCS if the container is ineed a VM
		isHypervContainer, err := testutil.HyperVContainer(inspect)
		assert.NilError(t, err)
		assert.Assert(t, isHypervContainer, true)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", testutil.Identifier(t))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "should fail to remove when still running",
			NoParallel:  true,
			Command:     test.Command("rm", testutil.Identifier(t)),
			Expected:    test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "should kill the container",
			NoParallel:  true,
			Command:     test.Command("kill", testutil.Identifier(t)),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "should remove the container when terminated",
			NoParallel:  true,
			Command:     test.Command("rm", testutil.Identifier(t)),
			Expected:    test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
	}

	testCase.Run(t)
}
