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
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposeBuild(t *testing.T) {
	dockerfile := "FROM " + testutil.AlpineImage

	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.Build

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// Make sure we shard the image name to something unique to the test to avoid conflicts with other tests
		imageSvc0 := data.Identifier("svc0")
		imageSvc1 := data.Identifier("svc1")

		// We are not going to run them, so, ports conflicts should not matter here
		dockerComposeYAML := fmt.Sprintf(`
services:
  svc0:
    build: .
    image: %s
    ports:
    - 8080:80
    depends_on:
    - svc1
  svc1:
    build: .
    image: %s
    ports:
    - 8081:80
`, imageSvc0, imageSvc1)

		data.Temp().Save(dockerComposeYAML, "compose.yaml")
		data.Temp().Save(dockerfile, "Dockerfile")

		data.Labels().Set("composeYaml", data.Temp().Path("compose.yaml"))
		data.Labels().Set("imageSvc0", imageSvc0)
		data.Labels().Set("imageSvc1", imageSvc1)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "build svc0",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("compose", "-f", data.Labels().Get("composeYaml"), "build", "svc0")
			},

			Command: test.Command("images"),

			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.All(
						expect.Contains(data.Labels().Get("imageSvc0")),
						expect.DoesNotContain(data.Labels().Get("imageSvc1")),
					),
				}
			},
		},
		{
			Description: "build svc0 and svc1",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("compose", "-f", data.Labels().Get("composeYaml"), "build", "svc0", "svc1")
			},

			Command: test.Command("images"),

			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.Contains(data.Labels().Get("imageSvc0"), data.Labels().Get("imageSvc1")),
				}
			},
		},
		{
			Description: "build no arg",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "build")
			},

			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
		},
		{
			Description: "build bogus",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command(
					"compose",
					"-f",
					data.Labels().Get("composeYaml"),
					"build",
					"svc0",
					"svc100",
				)
			},

			Expected: test.Expects(1, []error{errors.New("no such service: svc100")}, nil),
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("imageSvc0") != "" {
			helpers.Anyhow("rmi", data.Labels().Get("imageSvc0"), data.Labels().Get("imageSvc1"))
		}
	}

	testCase.Run(t)
}
