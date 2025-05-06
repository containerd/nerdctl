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
				expect.JSON([]composeContainerPrintable{}, func(printables []composeContainerPrintable, s string, t tig.T) {
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
				expect.JSON([]composeContainerPrintable{}, func(printables []composeContainerPrintable, s string, t tig.T) {
					assert.Equal(t, len(printables), 1)
				}),
				expect.Contains(`"ContainerName":"wordpress"`),
			)),
		},
	}

	testCase.Run(t)
}
