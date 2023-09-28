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

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/containerd/nerdctl/pkg/tabutil"
	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestComposePs(t *testing.T) {
	base := testutil.NewBase(t)
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

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
  alpine:
    image: %s
    container_name: alpine_container

volumes:
  wordpress:
  db:
`, testutil.WordpressImage, testutil.MariaDBImage, testutil.AlpineImage)
	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	assertHandler := func(expectedName, expectedImage string) func(stdout string) error {
		return func(stdout string) error {
			lines := strings.Split(strings.TrimSpace(stdout), "\n")
			if len(lines) < 2 {
				return fmt.Errorf("expected at least 2 lines, got %d", len(lines))
			}

			tab := tabutil.NewReader("NAME\tIMAGE\tCOMMAND\tSERVICE\tSTATUS\tPORTS")
			err := tab.ParseHeader(lines[0])
			if err != nil {
				return fmt.Errorf("failed to parse header: %v", err)
			}

			container, _ := tab.ReadRow(lines[1], "NAME")
			assert.Equal(t, container, expectedName)

			image, _ := tab.ReadRow(lines[1], "IMAGE")
			assert.Equal(t, image, expectedImage)

			return nil
		}

	}

	time.Sleep(3 * time.Second)
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "wordpress").AssertOutWithFunc(assertHandler("wordpress_container", testutil.WordpressImage))
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "db").AssertOutWithFunc(assertHandler("db_container", testutil.MariaDBImage))
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps").AssertOutNotContains(testutil.AlpineImage)
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "alpine", "-a").AssertOutWithFunc(assertHandler("alpine_container", testutil.AlpineImage))
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "-a", "--filter", "status=exited").AssertOutWithFunc(assertHandler("alpine_container", testutil.AlpineImage))
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "--services", "-a").AssertOutContainsAll("wordpress\n", "db\n", "alpine\n")
}

func TestComposePsJSON(t *testing.T) {
	// docker parses unknown 'format' as a Go template and won't output an error
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  wordpress:
    image: %s
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

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	assertHandler := func(svc string, count int, fields ...string) func(stdout string) error {
		return func(stdout string) error {
			// 1. check json output can be unmarshalled back to printables.
			var printables []composeContainerPrintable
			if err := json.Unmarshal([]byte(stdout), &printables); err != nil {
				return fmt.Errorf("[service: %s]failed to unmarshal json output from `compose ps`: %s", svc, stdout)
			}
			// 2. check #printables matches expected count.
			if len(printables) != count {
				return fmt.Errorf("[service: %s]unmarshal generates %d printables, expected %d: %s", svc, len(printables), count, stdout)
			}
			// 3. check marshalled json string has all expected substrings.
			for _, field := range fields {
				if !strings.Contains(stdout, field) {
					return fmt.Errorf("[service: %s]marshalled json output doesn't have expected string (%s): %s", svc, field, stdout)
				}
			}
			return nil
		}
	}

	// check other formats are not supported
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "--format", "yaml").AssertFail()
	// check all services are up (can be marshalled and unmarshalled) and check Image field exists
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "--format", "json").
		AssertOutWithFunc(assertHandler("all", 2, `"Service":"wordpress"`, `"Service":"db"`,
			fmt.Sprintf(`"Image":"%s"`, testutil.WordpressImage), fmt.Sprintf(`"Image":"%s"`, testutil.MariaDBImage)))
	// check wordpress is running
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "--format", "json", "wordpress").
		AssertOutWithFunc(assertHandler("wordpress", 1, `"Service":"wordpress"`, `"State":"running"`, `"TargetPort":80`, `"PublishedPort":8080`))
	// check wordpress is stopped
	base.ComposeCmd("-f", comp.YAMLFullPath(), "stop", "wordpress").AssertOK()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "--format", "json", "wordpress", "-a").
		AssertOutWithFunc(assertHandler("wordpress", 1, `"Service":"wordpress"`, `"State":"exited"`))
	// check wordpress is removed
	base.ComposeCmd("-f", comp.YAMLFullPath(), "rm", "-f", "wordpress").AssertOK()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "--format", "json", "wordpress").
		AssertOutWithFunc(assertHandler("wordpress", 0))
}
