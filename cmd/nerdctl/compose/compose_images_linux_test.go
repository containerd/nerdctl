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
	"strings"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestComposeImages(t *testing.T) {
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

	wordpressImageName, _ := referenceutil.Parse(testutil.WordpressImage)
	dbImageName, _ := referenceutil.Parse(testutil.MariaDBImage)

	// check one service image
	base.ComposeCmd("-f", comp.YAMLFullPath(), "images", "db").AssertOutContains(dbImageName.Name())
	base.ComposeCmd("-f", comp.YAMLFullPath(), "images", "db").AssertOutNotContains(wordpressImageName.Name())

	// check all service images
	base.ComposeCmd("-f", comp.YAMLFullPath(), "images").AssertOutContains(dbImageName.Name())
	base.ComposeCmd("-f", comp.YAMLFullPath(), "images").AssertOutContains(wordpressImageName.Name())
}

func TestComposeImagesJson(t *testing.T) {
	base := testutil.NewBase(t)
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  wordpress:
    image: %s
    container_name: wordpress
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
				return fmt.Errorf("[service: %s]failed to unmarshal json output from `compose images`: %s", svc, stdout)
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
	base.ComposeCmd("-f", comp.YAMLFullPath(), "images", "--format", "yaml").AssertFail()
	// check all services are up (can be marshalled and unmarshalled)
	base.ComposeCmd("-f", comp.YAMLFullPath(), "images", "--format", "json").
		AssertOutWithFunc(assertHandler("all", 2, `"ContainerName":"wordpress"`, `"ContainerName":"db"`))

	base.ComposeCmd("-f", comp.YAMLFullPath(), "images", "--format", "json", "wordpress").
		AssertOutWithFunc(assertHandler("wordpress", 1, `"ContainerName":"wordpress"`))
}
