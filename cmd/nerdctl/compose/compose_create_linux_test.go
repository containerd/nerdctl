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
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposeCreate(t *testing.T) {
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
`, testutil.AlpineImage)

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		compYamlPath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		data.Labels().Set("composeYaml", compYamlPath)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "`compose create` should work",
			// These are sequential
			NoParallel: true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "create")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "`compose create` should have created service container (in `created` status)",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "ps", "svc0", "-a")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, func(stdout, info string, t *testing.T) {
				assert.Assert(t,
					strings.Contains(stdout, "created") || strings.Contains(stdout, "Created"),
					"stdout should contain `created`")
			}),
		},
		{
			Description: "`created container can be started by `compose start`",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "start")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
	}

	testCase.Run(t)
}

func TestComposeCreateDependency(t *testing.T) {
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
    depends_on:
    - "svc1"
  svc1:
    image: %s
`, testutil.CommonImage, testutil.CommonImage)

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		compYamlPath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		data.Labels().Set("composeYaml", compYamlPath)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down", "-v")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "`compose create` should work",
			// These are sequential
			NoParallel: true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "create")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "`compose create` should have created svc0 (in `created` status)",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "ps", "svc0", "-a")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, func(stdout, info string, t *testing.T) {
				assert.Assert(t,
					strings.Contains(stdout, "created") || strings.Contains(stdout, "Created"),
					"stdout should contain `created`")
			}),
		},
		{
			Description: "`compose create` should have created svc1 (in `created` status)",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "ps", "svc1", "-a")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, func(stdout, info string, t *testing.T) {
				assert.Assert(t,
					strings.Contains(stdout, "created") || strings.Contains(stdout, "Created"),
					"stdout should contain `created`")
			}),
		},
	}

	testCase.Run(t)
}

func TestComposeCreatePull(t *testing.T) {

	base := testutil.NewBase(t)
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
`, testutil.AlpineImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	// `compose create --pull never` should fail: no such image
	base.Cmd("rmi", "-f", testutil.AlpineImage).Run()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "create", "--pull", "never").AssertFail()
	// `compose create --pull missing(default)|always` should succeed: image is pulled and container is created
	base.Cmd("rmi", "-f", testutil.AlpineImage).Run()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "create").AssertOK()
	base.Cmd("rmi", "-f", testutil.AlpineImage).Run()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "create", "--pull", "always").AssertOK()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "svc0", "-a").AssertOutContainsAny("Created", "created")
}

func TestComposeCreateBuild(t *testing.T) {
	const imageSvc0 = "composebuild_svc0"

	dockerComposeYAML := fmt.Sprintf(`
services:
  svc0:
    build: .
    image: %s
`, imageSvc0)

	dockerfile := fmt.Sprintf(`FROM %s`, testutil.AlpineImage)

	testutil.RequiresBuild(t)
	testutil.RegisterBuildCacheCleanup(t)
	base := testutil.NewBase(t)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	comp.WriteFile("Dockerfile", dockerfile)
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	defer base.Cmd("rmi", imageSvc0).Run()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	// `compose create --no-build` should fail if service image needs build
	base.ComposeCmd("-f", comp.YAMLFullPath(), "create", "--no-build").AssertFail()
	// `compose create --build` should succeed: image is built and container is created
	base.ComposeCmd("-f", comp.YAMLFullPath(), "create", "--build").AssertOK()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "images", "svc0").AssertOutContains(imageSvc0)
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "svc0", "-a").AssertOutContainsAny("Created", "created")
}
