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

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestCommit(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "with pause",
			Require:     nerdtest.CGroup,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
				helpers.Anyhow("rmi", "-f", data.Identifier())
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", "infinity")
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
				helpers.Ensure("exec", data.Identifier(), "sh", "-euxc", `echo hello-test-commit > /foo`)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				helpers.Ensure(
					"commit",
					"-c", `CMD ["/foo"]`,
					"-c", `ENTRYPOINT ["cat"]`,
					"--pause=true",
					data.Identifier(), data.Identifier())
				return helpers.Command("run", "--rm", data.Identifier())
			},
			Expected: test.Expects(0, nil, test.Equals("hello-test-commit\n")),
		},
		{
			Description: "no pause",
			Require:     test.Not(test.Windows),
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
				helpers.Anyhow("rmi", "-f", data.Identifier())
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", "infinity")
				nerdtest.EnsureContainerStarted(helpers, data.Identifier())
				helpers.Ensure("exec", data.Identifier(), "sh", "-euxc", `echo hello-test-commit > /foo`)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				helpers.Ensure(
					"commit",
					"-c", `CMD ["/foo"]`,
					"-c", `ENTRYPOINT ["cat"]`,
					"--pause=false",
					data.Identifier(), data.Identifier())
				return helpers.Command("run", "--rm", data.Identifier())
			},
			Expected: test.Expects(0, nil, test.Equals("hello-test-commit\n")),
		},
	}

	testCase.Run(t)
}
