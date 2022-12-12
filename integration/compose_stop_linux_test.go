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

package integration

import (
	"fmt"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestComposeStop(t *testing.T) {
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

	upAssertHandler := func(svc string) func(stdout string) error {
		return func(stdout string) error {
			// Docker Compose v1: "Up", v2: "running"
			if !strings.Contains(stdout, "Up") && !strings.Contains(stdout, "running") {
				return fmt.Errorf("service \"%s\" must have been still running", svc)
			}
			return nil
		}
	}

	exitAssertHandler := func(svc string) func(stdout string) error {
		return func(stdout string) error {
			// Docker Compose v1: "Exit code", v2: "exited (code)"
			if !strings.Contains(stdout, "Exit") && !strings.Contains(stdout, "exited") {
				return fmt.Errorf("service \"%s\" must have exited", svc)
			}
			return nil
		}
	}

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	// stop should (only) stop the given service.
	base.ComposeCmd("-f", comp.YAMLFullPath(), "stop", "db").AssertOK()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "db").AssertOutWithFunc(exitAssertHandler("db"))
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "wordpress").AssertOutWithFunc(upAssertHandler("wordpress"))

	// `--timeout` arg should work properly.
	base.ComposeCmd("-f", comp.YAMLFullPath(), "stop", "--timeout", "5", "wordpress").AssertOK()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "wordpress").AssertOutWithFunc(exitAssertHandler("wordpress"))

}
