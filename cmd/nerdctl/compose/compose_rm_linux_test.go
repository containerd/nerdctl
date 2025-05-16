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

func TestComposeRemove(t *testing.T) {
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

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down")
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save(dockerComposeYAML, "compose.yaml")
		helpers.Ensure("compose", "-f", data.Temp().Path("compose.yaml"), "up", "-d")
		data.Labels().Set("yamlPath", data.Temp().Path("compose.yaml"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "All services are still up",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("yamlPath"), "rm", "-f")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout, info string, t *testing.T) {
						wp := helpers.Capture("compose", "-f", data.Labels().Get("yamlPath"), "ps", "wordpress")
						db := helpers.Capture("compose", "-f", data.Labels().Get("yamlPath"), "ps", "db")
						comp := expect.Match(regexp.MustCompile("Up|running"))
						comp(wp, "", t)
						comp(db, "", t)
					},
				}
			},
		},
		{
			Description: "Remove stopped service",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				helpers.Ensure("compose", "-f", data.Labels().Get("yamlPath"), "stop", "wordpress")
				return helpers.Command("compose", "-f", data.Labels().Get("yamlPath"), "rm", "-f")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout, info string, t *testing.T) {
						wp := helpers.Capture("compose", "-f", data.Labels().Get("yamlPath"), "ps", "wordpress")
						db := helpers.Capture("compose", "-f", data.Labels().Get("yamlPath"), "ps", "db")
						expect.DoesNotContain("wordpress")(wp, "", t)
						expect.Match(regexp.MustCompile("Up|running"))(db, "", t)
					},
				}
			},
		},
		{
			Description: "Remove all services with stop",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("yamlPath"), "rm", "-f", "-s")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout, info string, t *testing.T) {
						db := helpers.Capture("compose", "-f", data.Labels().Get("yamlPath"), "ps", "db")
						expect.DoesNotContain("db")(db, "", t)
					},
				}
			},
		},
	}

	testCase.Run(t)
}
