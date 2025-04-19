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

func TestDiff(t *testing.T) {
	testCase := nerdtest.Setup()

	// It is unclear why this is failing with docker when run in parallel
	// Obviously some other container test is interfering
	if nerdtest.IsDocker() {
		testCase.NoParallel = true
	}

	testCase.Require = require.Not(require.Windows)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage,
			"sh", "-euxc", "touch /a; touch /bin/b; rm /bin/base64")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("diff", data.Identifier())
	}

	testCase.Expected = test.Expects(
		0,
		nil,
		expect.Contains(
			"A /a",
			"C /bin",
			"A /bin/b",
			"D /bin/base64"),
	)

	testCase.Run(t)
}
