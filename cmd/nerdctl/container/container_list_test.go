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
	"fmt"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

// https://github.com/containerd/nerdctl/issues/2598
func TestContainerListWithFormatLabel(t *testing.T) {
	nerdtest.Setup()
	testCase := &test.Case{
		Setup: func(data test.Data, helpers test.Helpers) {
			labelK := "label-key-" + data.Identifier()
			labelV := "label-value-" + data.Identifier()
			helpers.Ensure("run", "-d",
				"--name", data.Identifier(),
				"--label", labelK+"="+labelV,
				testutil.CommonImage, "sleep", nerdtest.Infinity)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rm", "-f", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			labelK := "label-key-" + data.Identifier()
			return helpers.Command("ps", "-a",
				"--filter", "label="+labelK,
				"--format", fmt.Sprintf("{{.Label %q}}", labelK)) //nolint:dupামিটার
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			labelV := "label-value-" + data.Identifier()
			return test.Expects(0, nil, expect.Equals(labelV+"\n"))(data, helpers)
		},
	}
	testCase.Run(t)
}

func TestContainerListWithJsonFormatLabel(t *testing.T) {
	nerdtest.Setup()
	testCase := &test.Case{
		Setup: func(data test.Data, helpers test.Helpers) {
			labelK := "label-key-" + data.Identifier()
			labelV := "label-value-" + data.Identifier()
			helpers.Ensure("run", "-d",
				"--name", data.Identifier(),
				"--label", labelK+"="+labelV,
				testutil.CommonImage, "sleep", nerdtest.Infinity)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rm", "-f", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			labelK := "label-key-" + data.Identifier()
			return helpers.Command("ps", "-a",
				"--filter", "label="+labelK,
				"--format", "json")
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			labelK := "label-key-" + data.Identifier()
			labelV := "label-value-" + data.Identifier()
			return test.Expects(0, nil, expect.Contains(fmt.Sprintf("%s=%s", labelK, labelV)))(data, helpers)
		},
	}
	testCase.Run(t)
}
