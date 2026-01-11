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

	testCase.SubTests = []*test.Case{
		{
			Description: "pause svc0 should only pause its own container",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("compose", "-f", data.Labels().Get("composeYAML"), "pause", "svc0")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "ps", "svc0", "-a")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Match(regexp.MustCompile("Paused|paused"))),
		},
		{
			Description: "svc1 should still be running",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "ps", "svc1")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Match(regexp.MustCompile("Up|running"))),
		},
		{
			Description: "unpause svc0 should recover the paused container",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("compose", "-f", data.Labels().Get("composeYAML"), "unpause", "svc0")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "ps", "svc0")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Match(regexp.MustCompile("Up|running"))),
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("composeYAML") != "" {
			helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
		}
	}

	testCase.Run(t)
}
