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
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposeLs(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Private
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		composeYAML := fmt.Sprintf(`
services:
  first:
    image: %s
    network_mode: none
    command: ["sleep", "infinity"]
  second:
    image: %s
    network_mode: none
    command: ["sleep", "infinity"]
`, testutil.CommonImage, testutil.CommonImage)
		path := data.Temp().Save(composeYAML, "project.yaml")
		data.Labels().Set("composeYAML", path)
		data.Labels().Set("runningProject", data.Identifier("running"))
		data.Labels().Set("createdProject", data.Identifier("created"))
		helpers.Ensure("compose", "-p", data.Labels().Get("runningProject"), "-f", path, "up", "-d")
		helpers.Ensure("compose", "-p", data.Labels().Get("createdProject"), "-f", path, "create")
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if path := data.Labels().Get("composeYAML"); path != "" {
			for _, key := range []string{"runningProject", "createdProject"} {
				helpers.Anyhow("compose", "-p", data.Labels().Get(key), "-f", path, "down", "-v")
			}
		}
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "lists running projects without loading a compose file",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command("compose", "ls", "--format=json")
				cmd.WithCwd(filepath.Dir(data.Labels().Get("composeYAML")))
				cmd.Setenv("COMPOSE_FILE", data.Temp().Path("missing.yaml"))
				return cmd
			},
			Expected: func(data test.Data, _ test.Helpers) *test.Expected {
				return &test.Expected{ExitCode: 0, Output: func(stdout string, t tig.T) {
					var projects []projectSummary
					assert.NilError(t, json.Unmarshal([]byte(stdout), &projects))
					assert.DeepEqual(t, projects, []projectSummary{{
						Name:        data.Labels().Get("runningProject"),
						Status:      "running(2)",
						ConfigFiles: data.Labels().Get("composeYAML"),
					}})
				}}
			},
		},
		{
			Description: "includes created projects and their configuration paths with all",
			Command: func(_ test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "ls", "--all", "--format=json")
			},
			Expected: func(data test.Data, _ test.Helpers) *test.Expected {
				return &test.Expected{ExitCode: 0, Output: func(stdout string, t tig.T) {
					var projects []projectSummary
					assert.NilError(t, json.Unmarshal([]byte(stdout), &projects))
					want := []projectSummary{
						{Name: data.Labels().Get("runningProject"), Status: "running(2)", ConfigFiles: data.Labels().Get("composeYAML")},
						{Name: data.Labels().Get("createdProject"), Status: "created(2)", ConfigFiles: data.Labels().Get("composeYAML")},
					}
					sort.Slice(want, func(i, j int) bool { return want[i].Name < want[j].Name })
					assert.DeepEqual(t, projects, want)
				}}
			},
		},
		{
			Description: "filters by project name",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				filter := "name=^" + regexp.QuoteMeta(data.Labels().Get("createdProject")) + "$"
				return helpers.Command("compose", "ls", "--all", "--quiet", "--filter", filter)
			},
			Expected: func(data test.Data, _ test.Helpers) *test.Expected {
				return &test.Expected{ExitCode: 0, Output: expect.Equals(data.Labels().Get("createdProject") + "\n")}
			},
		},
		{
			Description: "quiet output contains sorted project names only",
			Command: func(_ test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "ls", "--all", "--quiet")
			},
			Expected: func(data test.Data, _ test.Helpers) *test.Expected {
				names := []string{data.Labels().Get("runningProject"), data.Labels().Get("createdProject")}
				sort.Strings(names)
				return &test.Expected{ExitCode: 0, Output: expect.Equals(strings.Join(names, "\n") + "\n")}
			},
		},
	}
	testCase.Run(t)
}

func TestComposeLsStoppedProject(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = nerdtest.Private
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		composeYAML := fmt.Sprintf(`
services:
  first:
    image: %s
    network_mode: none
    command: ["sleep", "infinity"]
`, testutil.CommonImage)
		path := data.Temp().Save(composeYAML, "project.yaml")
		project := data.Identifier()
		data.Labels().Set("composeYAML", path)
		data.Labels().Set("project", project)
		helpers.Ensure("compose", "-p", project, "-f", path, "up", "-d")
		helpers.Ensure("compose", "-p", project, "-f", path, "stop")
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if path := data.Labels().Get("composeYAML"); path != "" {
			helpers.Anyhow("compose", "-p", data.Labels().Get("project"), "-f", path, "down", "-v")
		}
	}
	testCase.SubTests = []*test.Case{
		{
			Description: "hides a stopped project by default",
			Command: func(_ test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "ls", "--format=json")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("[]\n")),
		},
		{
			Description: "all includes a stopped project with its exited status",
			Command: func(_ test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "ls", "--all", "--format=json")
			},
			Expected: func(data test.Data, _ test.Helpers) *test.Expected {
				return &test.Expected{ExitCode: 0, Output: func(stdout string, t tig.T) {
					var projects []projectSummary
					assert.NilError(t, json.Unmarshal([]byte(stdout), &projects))
					assert.DeepEqual(t, projects, []projectSummary{{
						Name:        data.Labels().Get("project"),
						Status:      "exited(1)",
						ConfigFiles: data.Labels().Get("composeYAML"),
					}})
				}}
			},
		},
	}
	testCase.Run(t)
}
