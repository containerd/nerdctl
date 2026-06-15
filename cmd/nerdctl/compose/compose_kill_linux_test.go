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

func TestComposeKill(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		dockerComposeYAML := fmt.Sprintf(`
services:

  wordpress:
    image: %s
    environment:
      WORDPRESS_DB_HOST: db
      WORDPRESS_DB_USER: exampleuser
      WORDPRESS_DB_PASSWORD: examplepass
      WORDPRESS_DB_NAME: exampledb
    volumes:
      - wordpress:/var/www/html

  db:
    image: %s
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

		composePath := data.Temp().Save(dockerComposeYAML, "compose.yaml")

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		wordpressContainerName := serviceparser.DefaultContainerName(projectName, "wordpress", "1")
		dbContainerName := serviceparser.DefaultContainerName(projectName, "db", "1")

		data.Labels().Set("composeYAML", composePath)
		data.Labels().Set("wordpressContainer", wordpressContainerName)
		data.Labels().Set("dbContainer", dbContainerName)

		helpers.Ensure("compose", "-f", composePath, "up", "-d")
		nerdtest.EnsureContainerStarted(helpers, wordpressContainerName)
		nerdtest.EnsureContainerStarted(helpers, dbContainerName)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "kill db container and exit with 137",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("compose", "-f", data.Labels().Get("composeYAML"), "kill", "db")
				nerdtest.EnsureContainerExited(helpers, data.Labels().Get("dbContainer"), expect.ExitCodeSigkill)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "ps", "db", "-a")
			},
			// Docker Compose v1: "Exit 137", v2: "exited (137)"
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Match(regexp.MustCompile(` 137|\(137\)`))),
		},
		{
			Description: "wordpress container is still running",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "ps", "wordpress")
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
