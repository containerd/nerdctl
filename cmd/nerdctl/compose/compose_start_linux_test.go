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
	"regexp"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposeStart(t *testing.T) {
	var dockerComposeYAML = fmt.Sprintf(`
services:
  svc0:
    image: %s
    command: "sleep infinity"
  svc1:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage, testutil.CommonImage)

	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down")
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save(dockerComposeYAML, "compose.yaml")
		helpers.Ensure("compose", "-f", data.Temp().Path("compose.yaml"), "up", "-d")
		helpers.Ensure("compose", "-f", data.Temp().Path("compose.yaml"), "start")
		helpers.Ensure("compose", "-f", data.Temp().Path("compose.yaml"), "stop", "--timeout", "1", "svc0")
		helpers.Ensure("compose", "-f", data.Temp().Path("compose.yaml"), "kill", "svc1")
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("compose", "-f", data.Temp().Path("compose.yaml"), "start")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Errors:   nil,
			Output: func(stdout, info string, t *testing.T) {
				svc0 := helpers.Capture("compose", "-f", data.Temp().Path("compose.yaml"), "ps", "svc0")
				svc1 := helpers.Capture("compose", "-f", data.Temp().Path("compose.yaml"), "ps", "svc1")
				comp := expect.Match(regexp.MustCompile("Up|running"))
				comp(svc0, "", t)
				comp(svc1, "", t)
			},
		}
	}

	testCase.Run(t)
}

func TestComposeStartFailWhenServicePause(t *testing.T) {
	var dockerComposeYAML = fmt.Sprintf(`
services:
  svc0:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage)

	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.CGroup

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down")
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save(dockerComposeYAML, "compose.yaml")
		helpers.Ensure("compose", "-f", data.Temp().Path("compose.yaml"), "up", "-d")
		helpers.Ensure("compose", "-f", data.Temp().Path("compose.yaml"), "pause", "svc0")
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("compose", "-f", data.Temp().Path("compose.yaml"), "start")
	}

	testCase.Expected = test.Expects(expect.ExitCodeGenericFail, nil, nil)

	testCase.Run(t)
}
