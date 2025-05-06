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

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposeConfig(t *testing.T) {
	const dockerComposeYAML = `
services:
  hello:
    image: alpine:3.13
`
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save(dockerComposeYAML, "compose.yaml")
		data.Labels().Set("composeYaml", data.Temp().Path("compose.yaml"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "config contains service name",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "config")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("hello:")),
		},
		{
			Description: "config --services is exactly service name",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command(
					"compose",
					"-f",
					data.Labels().Get("composeYaml"),
					"config",
					"--services",
				)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("hello\n")),
		},
		{
			Description: "config --hash=* contains service name",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "config", "--hash=*")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("hello")),
		},
	}

	testCase.Run(t)
}

func TestComposeConfigWithPrintServiceHash(t *testing.T) {
	const dockerComposeYAML = `
services:
  hello:
    image: alpine:%s
`
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save(fmt.Sprintf(dockerComposeYAML, "3.13"), "compose.yaml")

		hash := helpers.Capture(
			"compose",
			"-f",
			data.Temp().Path("compose.yaml"),
			"config",
			"--hash=hello",
		)

		data.Labels().Set("hash", hash)

		data.Temp().Save(fmt.Sprintf(dockerComposeYAML, "3.14"), "compose.yaml")
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command(
			"compose",
			"-f",
			data.Temp().Path("compose.yaml"),
			"config",
			"--hash=hello",
		)
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Output: func(stdout, info string, t *testing.T) {
				assert.Assert(t, data.Labels().Get("hash") != stdout, "hash should be different")
			},
		}
	}

	testCase.Run(t)
}

func TestComposeConfigWithMultipleFile(t *testing.T) {
	const dockerComposeBase = `
services:
  hello1:
    image: alpine:3.13
`

	const dockerComposeTest = `
services:
  hello2:
    image: alpine:3.14
`

	const dockerComposeOverride = `
services:
  hello1:
    image: alpine:3.14
`

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save(dockerComposeBase, "compose.yaml")
		data.Temp().Save(dockerComposeTest, "docker-compose.test.yml")
		data.Temp().Save(dockerComposeOverride, "docker-compose.override.yml")

		data.Labels().Set("composeDir", data.Temp().Path())
		data.Labels().Set("composeYaml", data.Temp().Path("compose.yaml"))
		data.Labels().Set("composeYamlTest", data.Temp().Path("docker-compose.test.yml"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "config override",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command(
					"compose",
					"-f", data.Labels().Get("composeYaml"),
					"-f", data.Labels().Get("composeYamlTest"),
					"config",
				)
			},
			Expected: test.Expects(
				expect.ExitCodeSuccess,
				nil,
				expect.Contains("alpine:3.13", "alpine:3.14", "hello1", "hello2"),
			),
		},
		{
			Description: "project dir",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command(
					"compose",
					"--project-directory", data.Labels().Get("composeDir"), "config",
				)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("alpine:3.14")),
		},
		{
			Description: "project dir services",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command(
					"compose",
					"--project-directory", data.Labels().Get("composeDir"), "config", "--services",
				)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("hello1\n")),
		},
	}

	testCase.Run(t)
}

func TestComposeConfigWithComposeFileEnv(t *testing.T) {
	const dockerComposeBase = `
services:
  hello1:
    image: alpine:3.13
`

	const dockerComposeTest = `
services:
  hello2:
    image: alpine:3.14
`

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save(dockerComposeBase, "compose.yaml")
		data.Temp().Save(dockerComposeTest, "docker-compose.test.yml")

		data.Labels().Set("composeDir", data.Temp().Path())
		data.Labels().Set("composeYaml", data.Temp().Path("compose.yaml"))
		data.Labels().Set("composeYamlTest", data.Temp().Path("docker-compose.test.yml"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "env config",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command(
					"compose",
					"config",
				)
				cmd.Setenv("COMPOSE_FILE", data.Labels().Get("composeYaml")+","+data.Labels().Get("composeYamlTest"))
				cmd.Setenv("COMPOSE_PATH_SEPARATOR", ",")
				return cmd
			},
			Expected: test.Expects(
				expect.ExitCodeSuccess,
				nil,
				expect.Contains("alpine:3.13", "alpine:3.14", "hello1", "hello2"),
			),
		},
		{
			Description: "env with project dir",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				cmd := helpers.Command(
					"compose",
					"--project-directory", data.Labels().Get("composeDir"),
					"config",
				)
				cmd.Setenv("COMPOSE_FILE", data.Labels().Get("composeYaml")+","+data.Labels().Get("composeYamlTest"))
				cmd.Setenv("COMPOSE_PATH_SEPARATOR", ",")
				return cmd
			},
			Expected: test.Expects(
				expect.ExitCodeSuccess,
				nil,
				expect.Contains("alpine:3.13", "alpine:3.14", "hello1", "hello2"),
			),
		},
	}

	testCase.Run(t)
}

func TestComposeConfigWithEnvFile(t *testing.T) {
	const dockerComposeYAML = `
services:
  hello:
    image: ${image}
`
	const envFileContent = `
image: hello-world
`

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save(dockerComposeYAML, "compose.yaml")
		data.Temp().Save(envFileContent, "env")
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("compose",
			"-f", data.Temp().Path("compose.yaml"),
			"--env-file", data.Temp().Path("env"),
			"config",
		)
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("image: hello-world"))

	testCase.Run(t)
}
