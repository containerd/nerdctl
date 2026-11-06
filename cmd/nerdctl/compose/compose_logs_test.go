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

package compose

import (
	"fmt"
	"testing"
	"time"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposeLogsWithoutContainers(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save(fmt.Sprintf(`services:
  app:
    image: %s
`, testutil.CommonImage), "compose.yaml")
		data.Labels().Set("composeYaml", data.Temp().Path("compose.yaml"))
	}

	for _, args := range [][]string{
		{},
		{"app"},
		{"--follow"},
		{"--follow", "app"},
	} {
		testCase.SubTests = append(testCase.SubTests, &test.Case{
			Description: fmt.Sprintf("logs %v", args),
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmdArgs := []string{"compose", "-f", data.Labels().Get("composeYaml"), "-p", data.Identifier(), "logs"}
				cmd := helpers.Command(append(cmdArgs, args...)...)
				cmd.WithTimeout(30 * time.Second)
				return cmd
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("")),
		})
	}
	testCase.Run(t)
}
