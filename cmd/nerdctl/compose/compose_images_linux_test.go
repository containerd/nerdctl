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
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposeImages(t *testing.T) {
	var dockerComposeYAML = fmt.Sprintf(`
services:
  wordpress:
    image: %s
    container_name: wordpress
    environment:
      WORDPRESS_DB_HOST: db
      WORDPRESS_DB_USER: exampleuser
      WORDPRESS_DB_PASSWORD: examplepass
      WORDPRESS_DB_NAME: exampledb
    volumes:
      - wordpress:/var/www/html
  db:
    image: %s
    container_name: db
    environment:
      MYSQL_DATABASE: exampledb
      MYSQL_USER: exampleuser
      MYSQL_PASSWORD: examplepass
      MYSQL_RANDOM_ROOT_PASSWORD: '1'
    volumes:
      - db:/var/lib/mysql

volumes:
  wordpress:
  db:
`, testutil.WordpressImage, testutil.MariaDBImage)

	wordpressImageName, _ := referenceutil.Parse(testutil.WordpressImage)
	dbImageName, _ := referenceutil.Parse(testutil.MariaDBImage)

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save(dockerComposeYAML, "compose.yaml")
		data.Labels().Set("composeYaml", data.Temp().Path("compose.yaml"))
		helpers.Ensure("compose", "-f", data.Temp().Path("compose.yaml"), "up", "-d")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "images db",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "images", "db")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.All(
				expect.Contains(dbImageName.Name()),
				expect.DoesNotContain(wordpressImageName.Name()),
			)),
		},
		{
			Description: "images",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "images")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(dbImageName.Name(), wordpressImageName.Name())),
		},
		{
			Description: "images --format yaml",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "images", "--format", "yaml")
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
		{
			Description: "images --format json",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "images", "--format", "json")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.All(
				expect.JSON([]composeContainerPrintable{}, func(printables []composeContainerPrintable, t tig.T) {
					assert.Equal(t, len(printables), 2)
				}),
				expect.Contains(`"ContainerName":"wordpress"`, `"ContainerName":"db"`),
			)),
		},
		{
			Description: "images --format json wordpress",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "images", "--format", "json", "wordpress")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.All(
				expect.JSON([]composeContainerPrintable{}, func(printables []composeContainerPrintable, t tig.T) {
					assert.Equal(t, len(printables), 1)
				}),
				expect.Contains(`"ContainerName":"wordpress"`),
			)),
		},
	}

	testCase.Run(t)
}

func TestComposeImagesAfterRetag(t *testing.T) {
	testCase := nerdtest.Setup()
	// Docker does not support image inspect --mode native, which this test
	// uses to verify the manifest digest after retagging.
	testCase.Require = require.All(nerdtest.Private, require.Not(nerdtest.Docker))

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		imageName := data.Identifier("image")
		containerName := data.Identifier("container")
		helpers.Ensure("pull", testutil.AlpineImage)
		helpers.Ensure("pull", testutil.BusyboxImage)
		helpers.Ensure("tag", testutil.AlpineImage, imageName)
		data.Temp().Save(fmt.Sprintf(`services:
  test:
    image: %s
    container_name: %s
    network_mode: none
    command: ["sleep", "%s"]
`, imageName, containerName, nerdtest.Infinity), "compose.yaml")
		data.Labels().Set("composeYaml", data.Temp().Path("compose.yaml"))
		helpers.Ensure("compose", "-f", data.Temp().Path("compose.yaml"), "up", "-d")
		originalID := strings.TrimSpace(helpers.Capture("inspect", "--format", "{{.Image}}", containerName))
		data.Labels().Set("originalID", originalID)
		data.Labels().Set("originalShortID", strings.TrimPrefix(originalID, "sha256:")[:12])

		data.Labels().Set("originalTable", helpers.Capture("compose", "-f", data.Temp().Path("compose.yaml"), "images"))
		data.Labels().Set("originalJSON", helpers.Capture("compose", "-f", data.Temp().Path("compose.yaml"), "images", "--format", "json"))
		originalSize := strings.TrimSpace(helpers.Capture("image", "inspect", "--format", "{{.Size}}", testutil.AlpineImage))
		updatedSize := strings.TrimSpace(helpers.Capture("image", "inspect", "--format", "{{.Size}}", testutil.BusyboxImage))
		assert.Assert(helpers.T(), originalSize != updatedSize)

		// Retag to an image with a different rootfs so both ID and size must stay pinned.
		helpers.Ensure("tag", testutil.BusyboxImage, imageName)
		updatedID := strings.TrimSpace(helpers.Capture("image", "inspect", "--mode", "native", "--format", "{{.Image.Target.Digest}}", imageName))
		assert.Assert(helpers.T(), originalID != updatedID)
	}
	testCase.SubTests = []*test.Case{
		{
			Description: "table",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "images")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{ExitCode: expect.ExitCodeSuccess, Output: expect.Equals(data.Labels().Get("originalTable"))}
			},
		},
		{
			Description: "json",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "images", "--format", "json")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{ExitCode: expect.ExitCodeSuccess, Output: expect.Equals(data.Labels().Get("originalJSON"))}
			},
		},
		{
			Description: "quiet",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYaml"), "images", "--quiet")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{ExitCode: expect.ExitCodeSuccess, Output: expect.Equals(data.Labels().Get("originalShortID") + "\n")}
			},
		},
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down")
	}
	testCase.Run(t)
}
