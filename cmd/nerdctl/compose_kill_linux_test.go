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
	"testing"
	"time"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestComposeKill(t *testing.T) {
	// docker-compose v2 hides exited/killed containers in `compose ps`, and shows
	// them if `-a` is passed, which is not supported yet by `nerdctl compose`.
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

	base.ComposeCmd("-f", comp.YAMLFullPath(), "kill", "db").AssertOK()
	time.Sleep(3 * time.Second)
	// Docker Compose v1: "Exit 137", v2: "exited (137)"
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "db", "-a").AssertOutContainsAny(" 137", "(137)")
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "wordpress").AssertOutContainsAny("Up", "running")
}
