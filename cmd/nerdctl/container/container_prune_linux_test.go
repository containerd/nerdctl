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

	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestPruneContainer(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.Private

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("1"))
		helpers.Anyhow("rm", "-f", data.Identifier("2"))
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier("1"), "-v", "/anonymous", testutil.CommonImage, "sleep", nerdtest.Infinity)
		helpers.Ensure("exec", data.Identifier("1"), "touch", "/anonymous/foo")
		helpers.Ensure("create", "--name", data.Identifier("2"), testutil.CommonImage, "sleep", nerdtest.Infinity)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		helpers.Ensure("container", "prune", "-f")
		helpers.Ensure("inspect", data.Identifier("1"))
		helpers.Fail("inspect", data.Identifier("2"))
		// https://github.com/containerd/nerdctl/issues/3134
		helpers.Ensure("exec", data.Identifier("1"), "ls", "-lA", "/anonymous/foo")
		helpers.Ensure("kill", data.Identifier("1"))
		helpers.Ensure("container", "prune", "-f")
		return helpers.Command("inspect", data.Identifier("1"))
	}

	testCase.Expected = test.Expects(1, nil, nil)
}
