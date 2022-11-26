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
	"fmt"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
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

	ImageAssertHandler := func(svc string, image string, exist bool) func(stdout string) error {
		return func(stdout string) error {
			if strings.Contains(stdout, image) != exist {
				return fmt.Errorf("image %s from service %s: expect in output (%t), actual (%t)", image, svc, exist, !exist)
			}
			return nil
		}
	}

	wordpressImageName := strings.Split(testutil.WordpressImage, ":")[0]
	dbImageName := strings.Split(testutil.MariaDBImage, ":")[0]

	// check one service image
	base.ComposeCmd("-f", comp.YAMLFullPath(), "images", "db").AssertOutWithFunc(ImageAssertHandler("db", dbImageName, true))
	base.ComposeCmd("-f", comp.YAMLFullPath(), "images", "db").AssertOutWithFunc(ImageAssertHandler("wordpress", wordpressImageName, false))

	// check all service images
	base.ComposeCmd("-f", comp.YAMLFullPath(), "images").AssertOutWithFunc(ImageAssertHandler("db", dbImageName, true))
	base.ComposeCmd("-f", comp.YAMLFullPath(), "images").AssertOutWithFunc(ImageAssertHandler("wordpress", wordpressImageName, true))
}
