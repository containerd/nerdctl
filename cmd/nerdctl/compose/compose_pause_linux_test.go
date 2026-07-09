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
	"path/filepath"
	"regexp"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposePauseAndUnpause(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.CGroup

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		dockerComposeYAML := fmt.Sprintf(`
services:
  svc0:
    image: %s
    command: "sleep infinity"
  svc1:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage, testutil.CommonImage)

		composePath := data.Temp().Save(dockerComposeYAML, "compose.yaml")

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		svc0Container := serviceparser.DefaultContainerName(projectName, "svc0", "1")
		svc1Container := serviceparser.DefaultContainerName(projectName, "svc1", "1")

		data.Labels().Set("composeYAML", composePath)

		helpers.Ensure("compose", "-f", composePath, "up", "-d")
		nerdtest.EnsureContainerStarted(helpers, svc0Container)
		nerdtest.EnsureContainerStarted(helpers, svc1Container)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// pause a service should (only) pause its own container
		return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "pause", "svc0")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				svc0Paused := helpers.Capture("compose", "-f", data.Labels().Get("composeYAML"), "ps", "svc0", "-a")
				expect.Match(regexp.MustCompile("Paused|paused"))(svc0Paused, t)

				svc1Running := helpers.Capture("compose", "-f", data.Labels().Get("composeYAML"), "ps", "svc1")
				expect.Match(regexp.MustCompile("Up|running"))(svc1Running, t)

				// unpause should be able to recover the paused service container
				helpers.Ensure("compose", "-f", data.Labels().Get("composeYAML"), "unpause", "svc0")
				svc0Running := helpers.Capture("compose", "-f", data.Labels().Get("composeYAML"), "ps", "svc0")
				expect.Match(regexp.MustCompile("Up|running"))(svc0Running, t)
			},
		}
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("composeYAML") != "" {
			helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
		}
	}

	testCase.Run(t)
}
