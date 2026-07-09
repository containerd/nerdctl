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
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposeCreate(t *testing.T) {
	var dockerComposeYAML = fmt.Sprintf(`
services:
  svc0:
    image: %s
`, testutil.CommonImage)

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
			Expected: test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, t tig.T) {
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
			Expected: test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, t tig.T) {
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
			Expected: test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, t tig.T) {
				assert.Assert(t,
					strings.Contains(stdout, "created") || strings.Contains(stdout, "Created"),
					"stdout should contain `created`")
			}),
		},
	}

	testCase.Run(t)
}

func TestComposeCreatePull(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.NoParallel = true
	testCase.Require = nerdtest.Private

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		composeYAML := fmt.Sprintf(`
services:
  svc0:
    image: %s
`, testutil.CommonImage)

		composePath := data.Temp().Save(composeYAML, "compose.yaml")

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		data.Labels().Set("composeYAML", composePath)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "compose create --pull never fails when image missing",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("rmi", "-f", testutil.CommonImage)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "create", "--pull", "never")
			},
			Expected: test.Expects(1, nil, nil),
		},
		{
			Description: "compose create --pull missing (default) pulls and creates a container",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("rmi", "-f", testutil.CommonImage)
				helpers.Ensure("compose", "-f", data.Labels().Get("composeYAML"), "create")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "ps", "svc0", "-a")
			},
			Expected: test.Expects(0, nil, expect.Match(regexp.MustCompile(`Created|created`))),
		},
		{
			Description: "compose create --pull always pulls and creates a container",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("rmi", "-f", testutil.CommonImage)
				helpers.Ensure("compose", "-f", data.Labels().Get("composeYAML"), "create", "--pull", "always")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "ps", "svc0", "-a")
			},
			Expected: test.Expects(0, nil, expect.Match(regexp.MustCompile(`Created|created`))),
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("composeYAML") != "" {
			helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
		}
	}

	testCase.Run(t)
}

func TestComposeCreatePullInvalidOption(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		composeYAML := fmt.Sprintf(`
services:
  svc0:
    image: %s
`, testutil.CommonImage)

		composePath := data.Temp().Save(composeYAML, "compose.yaml")
		data.Labels().Set("composeYAML", composePath)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// nerver isn't never.
		return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "create", "--pull", "nerver")
	}

	testCase.Expected = test.Expects(1, []error{errors.New(`invalid --pull option \"nerver\"`)}, nil)

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if path := data.Labels().Get("composeYAML"); path != "" {
			helpers.Anyhow("compose", "-f", path, "down", "-v")
		}
	}

	testCase.Run(t)
}

func TestComposeCreateBuild(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.NoParallel = true
	testCase.Require = require.All(
		nerdtest.Private,
		nerdtest.Build,
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		imageSvc0 := data.Identifier("composebuild_svc0")
		composeYAML := fmt.Sprintf(`
services:
  svc0:
    build: .
    image: %s
`, imageSvc0)
		dockerfile := fmt.Sprintf(`FROM %s`, testutil.CommonImage)

		composePath := data.Temp().Save(composeYAML, "compose.yaml")
		data.Temp().Save(dockerfile, "Dockerfile")

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		data.Labels().Set("composeYAML", composePath)
		data.Labels().Set("imageName", imageSvc0)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "compose create --no-build fails when image needs to be built",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "create", "--no-build")
			},
			Expected: test.Expects(1, nil, nil),
		},
		{
			Description: "compose create --build builds image and creates container",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("compose", "-f", data.Labels().Get("composeYAML"), "create", "--build")
				helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "images", "svc0").Run(
					&test.Expected{
						ExitCode: 0,
						Output: func(stdout string, t tig.T) {
							assert.Assert(t, strings.Contains(stdout, data.Labels().Get("imageName")))
						},
					},
				)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "ps", "svc0", "-a")
			},
			Expected: test.Expects(0, nil, expect.Match(regexp.MustCompile(`Created|created`))),
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("composeYAML") != "" {
			helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
		}
		helpers.Anyhow("rmi", "-f", data.Labels().Get("imageName"))
		helpers.Anyhow("builder", "prune", "--all", "--force")
	}

	testCase.Run(t)
}

func TestComposeCreateWritesConfigHashLabel(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		var composeYAML = fmt.Sprintf(`
services:
  svc0:
    image: %s
`, testutil.CommonImage)
		composePath := data.Temp().Save(composeYAML, "compose.yaml")

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		data.Labels().Set("composeYAML", composePath)
		data.Labels().Set("containerName", serviceparser.DefaultContainerName(projectName, "svc0", "1"))

		helpers.Ensure("compose", "-f", composePath, "create")
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", "--format", "{{json .Config.Labels}}", data.Labels().Get("containerName"))
	}

	testCase.Expected = test.Expects(0, nil, expect.Contains("com.docker.compose.config-hash"))

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if path := data.Labels().Get("composeYAML"); path != "" {
			helpers.Anyhow("compose", "-f", path, "down", "-v")
		}
	}

	testCase.Run(t)
}
