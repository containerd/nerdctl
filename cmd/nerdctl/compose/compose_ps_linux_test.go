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
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/tabutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestComposePs(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.Private

	var dockerComposeYAML = fmt.Sprintf(`
services:
  wordpress:
    image: %s
    container_name: wordpress_container
    environment:
      WORDPRESS_DB_HOST: db
      WORDPRESS_DB_USER: exampleuser
      WORDPRESS_DB_PASSWORD: examplepass
      WORDPRESS_DB_NAME: exampledb
    volumes:
      - wordpress:/var/www/html
  db:
    image: %s
    container_name: db_container
    environment:
      MYSQL_DATABASE: exampledb
      MYSQL_USER: exampleuser
      MYSQL_PASSWORD: examplepass
      MYSQL_RANDOM_ROOT_PASSWORD: '1'
    volumes:
      - db:/var/lib/mysql
  alpine:
    image: %s
    container_name: alpine_container

volumes:
  wordpress:
  db:
`, testutil.WordpressImage, testutil.MariaDBImage, testutil.CommonImage)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		composePath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		data.Labels().Set("composeYAML", composePath)

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		helpers.Ensure("compose", "-f", composePath, "up", "-d")

		nerdtest.EnsureContainerStarted(helpers, "wordpress_container")
		nerdtest.EnsureContainerStarted(helpers, "db_container")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if path := data.Labels().Get("composeYAML"); path != "" {
			helpers.Anyhow("compose", "-f", path, "down", "-v")
		}
	}

	assertHandler := func(expectedName, expectedImage string) test.Comparator {
		return func(stdout string, t tig.T) {

			lines := strings.Split(strings.TrimSpace(stdout), "\n")
			assert.Assert(t, len(lines) >= 2)

			tab := tabutil.NewReader("NAME\tIMAGE\tCOMMAND\tSERVICE\tSTATUS\tPORTS")
			assert.NilError(t, tab.ParseHeader(lines[0]))

			container, _ := tab.ReadRow(lines[1], "NAME")
			assert.Equal(t, container, expectedName)

			image, _ := tab.ReadRow(lines[1], "IMAGE")
			assert.Equal(t, image, expectedImage)
		}
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "compose ps wordpress",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "ps", "wordpress")
			},
			Expected: test.Expects(0, nil,
				assertHandler("wordpress_container", testutil.WordpressImage),
			),
		},
		{
			Description: "compose ps db",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "ps", "db")
			},
			Expected: test.Expects(0, nil,
				assertHandler("db_container", testutil.MariaDBImage),
			),
		},
		{
			Description: "compose ps should not show alpine unless running",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "ps")
			},
			Expected: test.Expects(0, nil,
				expect.DoesNotContain(testutil.CommonImage),
			),
		},
		{
			Description: "compose ps alpine -a",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "ps", "alpine", "-a")
			},
			Expected: test.Expects(0, nil,
				assertHandler("alpine_container", testutil.CommonImage),
			),
		},
		{
			Description: "compose ps filter exited",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "ps", "-a", "--filter", "status=exited")
			},
			Expected: test.Expects(0, nil,
				assertHandler("alpine_container", testutil.CommonImage),
			),
		},
		{
			Description: "compose ps services",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "ps", "--services", "-a")
			},
			Expected: test.Expects(0, nil,
				expect.All(
					expect.Contains("wordpress\n"),
					expect.Contains("db\n"),
					expect.Contains("alpine\n"),
				),
			),
		},
	}

	testCase.Run(t)
}

func TestComposePsJSON(t *testing.T) {
	testCase := nerdtest.Setup()

	// docker parses unknown 'format' as a Go template and won't output an error
	testCase.Require = require.All(
		nerdtest.Private,
		require.Not(nerdtest.Docker),
	)

	var dockerComposeYAML = fmt.Sprintf(`
services:
  wordpress:
    image: %s
    container_name: wordpress_container
    ports:
      - 8080:80
    environment:
      WORDPRESS_DB_HOST: db
      WORDPRESS_DB_USER: exampleuser
      WORDPRESS_DB_PASSWORD: examplepass
      WORDPRESS_DB_NAME: exampledb
    volumes:
      - wordpress:/var/www/html
  db:
    image: %s
    container_name: db_container
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

	testCase.Setup = func(data test.Data, helpers test.Helpers) {

		composePath := data.Temp().Save(dockerComposeYAML, "compose.yaml")
		data.Labels().Set("composeYAML", composePath)

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		helpers.Ensure("compose", "-f", composePath, "up", "-d")

		nerdtest.EnsureContainerStarted(helpers, "wordpress_container")
		nerdtest.EnsureContainerStarted(helpers, "db_container")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if path := data.Labels().Get("composeYAML"); path != "" {
			helpers.Anyhow("compose", "-f", path, "down", "-v")
		}
	}

	assertHandler := func(svc string, count int, fields ...string) test.Comparator {
		return func(stdout string, t tig.T) {

			var printables []composeContainerPrintable
			// 1. check json output can be unmarshalled back to printables.
			assert.NilError(t, json.Unmarshal([]byte(stdout), &printables))
			// 2. check #printables matches expected count.
			assert.Equal(t, len(printables), count)
			// 3. check marshalled json string has all expected substrings.
			for _, field := range fields {
				assert.Assert(t, strings.Contains(stdout, field),
					fmt.Sprintf("[service: %s] expected %s in %s", svc, field, stdout))
			}
		}
	}

	testCase.SubTests = []*test.Case{
		{ // check other formats are not supported
			Description: "unsupported format should fail",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command(
					"compose",
					"-f", data.Labels().Get("composeYAML"),
					"ps",
					"--format", "yaml",
				)
			},
			Expected: test.Expects(1, nil, nil),
		},
		{ // check all services are up (can be marshalled and unmarshalled) and check Image field exists
			Description: "ps json all services",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "ps", "--format", "json")
			},
			Expected: test.Expects(0, nil,
				assertHandler("all", 2,
					`"Service":"wordpress"`,
					`"Service":"db"`,
					fmt.Sprintf(`"Image":"%s"`, testutil.WordpressImage),
					fmt.Sprintf(`"Image":"%s"`, testutil.MariaDBImage),
				),
			),
		},
		{ // check wordpress is running
			Description: "wordpress running",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"),
					"ps", "--format", "json", "wordpress")
			},
			Expected: test.Expects(0, nil,
				assertHandler("wordpress", 1,
					`"Service":"wordpress"`,
					`"State":"running"`,
					`"TargetPort":80`,
					`"PublishedPort":8080`,
				)),
		},
		{
			Description: "stop wordpress",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"),
					"stop", "wordpress")
			},
			Expected: test.Expects(0, nil, nil),
		},
		{ // check wordpress is stopped
			Description: "wordpress exited",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"),
					"ps", "--format", "json", "wordpress", "-a")
			},
			Expected: test.Expects(0, nil,
				assertHandler("wordpress", 1,
					`"Service":"wordpress"`,
					`"State":"exited"`,
				)),
		},
		{
			Description: "remove wordpress",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"),
					"rm", "-f", "wordpress")
			},
			Expected: test.Expects(0, nil, nil),
		},
		{ // check wordpress is removed
			Description: "wordpress removed",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"),
					"ps", "--format", "json", "wordpress")
			},
			Expected: test.Expects(0, nil,
				assertHandler("wordpress", 0),
			),
		},
	}

	testCase.Run(t)
}
