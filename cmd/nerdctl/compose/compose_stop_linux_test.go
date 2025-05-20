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
	"regexp"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposeStop(t *testing.T) {
	var dockerComposeYAML = fmt.Sprintf(`
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

	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save(dockerComposeYAML, "compose.yaml")
		helpers.Ensure("compose", "-f", data.Temp().Path("compose.yaml"), "up", "-d")
		data.Labels().Set("yamlPath", data.Temp().Path("compose.yaml"))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "stop db",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("compose", "-f", data.Labels().Get("yamlPath"), "stop", "db")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("yamlPath"), "ps", "db", "-a")
			},
			Expected: test.Expects(0, nil, expect.Match(regexp.MustCompile("Exit|exited"))),
		},
		{
			Description: "wordpress is still running",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("yamlPath"), "ps", "wordpress")
			},
			Expected: test.Expects(0, nil, expect.Match(regexp.MustCompile("Up|running"))),
		},
		{
			Description: "stop wordpress",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("compose", "-f", data.Labels().Get("yamlPath"), "stop", "--timeout", "5", "wordpress")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("yamlPath"), "ps", "wordpress", "-a")
			},
			Expected: test.Expects(0, nil, expect.Match(regexp.MustCompile("Exit|exited"))),
		},
	}

	testCase.Run(t)
}
