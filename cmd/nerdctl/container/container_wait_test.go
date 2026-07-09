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
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestWait(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("1"), data.Identifier("2"), data.Identifier("3"))
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier("1"), testutil.CommonImage)
		helpers.Ensure("run", "-d", "--name", data.Identifier("2"), testutil.CommonImage, "sleep", "1")
		helpers.Ensure("run", "-d", "--name", data.Identifier("3"), testutil.CommonImage, "sh", "-euxc", "sleep 5; exit 123")
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("wait", data.Identifier("1"), data.Identifier("2"), data.Identifier("3"))
	}

	testCase.Expected = test.Expects(0, nil, expect.Equals(`0
0
123
`))

	testCase.Run(t)
}
