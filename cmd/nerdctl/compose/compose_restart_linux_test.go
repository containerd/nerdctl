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
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposeRestart(t *testing.T) {
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
		helpers.Anyhow("compose", "-f", data.Labels().Get("yamlPath"), "down", "-v")
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "restart single service",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("compose", "-f", data.Labels().Get("yamlPath"), "stop", "db")
				ps := helpers.Capture("compose", "-f", data.Labels().Get("yamlPath"), "ps", "db", "-a")
				expect.Match(regexp.MustCompile("Exit|exited"))(ps, helpers.T())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("yamlPath"), "restart", "db")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						ps := helpers.Capture("compose", "-f", data.Labels().Get("yamlPath"), "ps", "db")
						expect.Match(regexp.MustCompile("Up|running"))(ps, t)
					},
				}
			},
		},
		{
			Description: "stop one service and restart all with timeout",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("compose", "-f", data.Labels().Get("yamlPath"), "stop", "db")
				ps := helpers.Capture("compose", "-f", data.Labels().Get("yamlPath"), "ps", "db", "-a")
				expect.Match(regexp.MustCompile("Exit|exited"))(ps, helpers.T())
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("yamlPath"), "restart", "--timeout", "5")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeSuccess,
					Output: func(stdout string, t tig.T) {
						db := helpers.Capture("compose", "-f", data.Labels().Get("yamlPath"), "ps", "db")
						wp := helpers.Capture("compose", "-f", data.Labels().Get("yamlPath"), "ps", "wordpress")
						comp := expect.Match(regexp.MustCompile("Up|running"))
						comp(db, t)
						comp(wp, t)
					},
				}
			},
		},
	}

	testCase.Run(t)
}
