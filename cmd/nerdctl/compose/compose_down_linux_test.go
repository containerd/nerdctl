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

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposeDownRemoveUsedNetwork(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		dockerComposeYAMLOrphan := fmt.Sprintf(`
services:
  test:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage)

		dockerComposeYAMLFull := fmt.Sprintf(`
%s
  orphan:
    image: %s
    command: "sleep infinity"
`, dockerComposeYAMLOrphan, testutil.CommonImage)

		composeOrphanPath := data.Temp().Save(dockerComposeYAMLOrphan, "compose-orphan.yaml")
		composeFullPath := data.Temp().Save(dockerComposeYAMLFull, "compose-full.yaml")

		projectName := data.Identifier("project")
		t.Logf("projectName=%q", projectName)

		testContainer := serviceparser.DefaultContainerName(projectName, "test", "1")
		orphanContainer := serviceparser.DefaultContainerName(projectName, "orphan", "1")

		data.Labels().Set("composeOrphan", composeOrphanPath)
		data.Labels().Set("composeFull", composeFullPath)
		data.Labels().Set("projectName", projectName)

		helpers.Ensure("compose", "-p", projectName, "-f", composeFullPath, "up", "-d")
		nerdtest.EnsureContainerStarted(helpers, testContainer)
		nerdtest.EnsureContainerStarted(helpers, orphanContainer)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("compose", "-p", data.Labels().Get("projectName"), "-f", data.Labels().Get("composeOrphan"), "down", "-v")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Errors: []error{
				fmt.Errorf("in use"),
			},
		}
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if composeFull := data.Labels().Get("composeFull"); composeFull != "" {
			helpers.Anyhow("compose", "-p", data.Labels().Get("projectName"), "-f", composeFull, "down", "--remove-orphans")
		}
	}

	testCase.Run(t)
}

func TestComposeDownRemoveOrphans(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		dockerComposeYAMLOrphan := fmt.Sprintf(`
services:
  test:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage)

		dockerComposeYAMLFull := fmt.Sprintf(`
%s
  orphan:
    image: %s
    command: "sleep infinity"
`, dockerComposeYAMLOrphan, testutil.CommonImage)

		composeOrphanPath := data.Temp().Save(dockerComposeYAMLOrphan, "compose-orphan.yaml")
		composeFullPath := data.Temp().Save(dockerComposeYAMLFull, "compose-full.yaml")

		projectName := data.Identifier("project")
		t.Logf("projectName=%q", projectName)

		testContainer := serviceparser.DefaultContainerName(projectName, "test", "1")
		orphanContainer := serviceparser.DefaultContainerName(projectName, "orphan", "1")

		data.Labels().Set("composeOrphan", composeOrphanPath)
		data.Labels().Set("composeFull", composeFullPath)
		data.Labels().Set("projectName", projectName)
		data.Labels().Set("orphanContainer", orphanContainer)

		helpers.Ensure("compose", "-p", projectName, "-f", composeFullPath, "up", "-d")
		nerdtest.EnsureContainerStarted(helpers, testContainer)
		nerdtest.EnsureContainerStarted(helpers, orphanContainer)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("compose", "-p", data.Labels().Get("projectName"), "-f", data.Labels().Get("composeOrphan"), "down", "--remove-orphans")
	}

	testCase.Expected = test.Expects(0, nil, nil)

	testCase.SubTests = []*test.Case{
		{
			Description: "orphan container removed",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-p", data.Labels().Get("projectName"), "-f", data.Labels().Get("composeFull"), "ps", "-a")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output:   expect.DoesNotContain(data.Labels().Get("orphanContainer")),
				}
			},
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if composeFull := data.Labels().Get("composeFull"); composeFull != "" {
			helpers.Anyhow("compose", "-p", data.Labels().Get("projectName"), "-f", composeFull, "down", "-v")
		}
	}

	testCase.Run(t)
}
