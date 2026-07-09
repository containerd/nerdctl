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

func TestStart(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("run", "-d",
				"--name", data.Identifier(),
				testutil.CommonImage)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rm", "-f", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("start", data.Identifier())
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return test.Expects(0, nil, expect.Contains(data.Identifier()))(data, helpers)
		},
	}
	testCase.Run(t)
}

func TestStartAttach(t *testing.T) {

	nerdtest.Setup()

	testCase := &test.Case{
		Require: require.Not(require.Windows),
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("run",
				"--name", data.Identifier(),
				testutil.CommonImage, "sh", "-euxc", "echo foo")
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rm", "-f", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("start", "-a", data.Identifier())
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return test.Expects(0, nil, expect.Contains("foo"))(data, helpers)
		},
	}
	testCase.Run(t)
}
