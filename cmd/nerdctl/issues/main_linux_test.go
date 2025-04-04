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

package issues

import (
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestMain(m *testing.M) {
	testutil.M(m)
}

// TestIssue108 tests https://github.com/containerd/nerdctl/issues/108
// ("`nerdctl run --net=host -it` fails while `nerdctl run -it --net=host` works")
func TestIssue108(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "-it --net=host",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "--quiet", "-it", "--rm", "--net=host", testutil.CommonImage, "echo", "this was always working")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(0, nil, expect.Equals("this was always working\r\n")),
		},
		{
			Description: "--net=host -it",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("run", "--quiet", "--rm", "--net=host", "-it", testutil.CommonImage, "echo", "this was not working due to issue #108")
				cmd.WithPseudoTTY()
				return cmd
			},
			Expected: test.Expects(0, nil, expect.Equals("this was not working due to issue #108\r\n")),
		},
	}

	testCase.Run(t)
}
