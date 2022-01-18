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
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"

	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/containerd/nerdctl/pkg/testutil/nettestutil"

	"gotest.tools/v3/assert"
)

func TestComposeUp(t *testing.T) {
	base := testutil.NewBase(t)
	testComposeUp(t, base, fmt.Sprintf(`
version: '3.1'

services:

  wordpress:
    image: %s
    restart: always
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
    restart: always
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
`, testutil.WordpressImage, testutil.MariaDBImage))
}

func testComposeUp(t *testing.T, base *testutil.Base, dockerComposeYAML string) {
	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()
	base.Cmd("volume", "inspect", fmt.Sprintf("%s_db", projectName)).AssertOK()
	base.Cmd("network", "inspect", fmt.Sprintf("%s_default", projectName)).AssertOK()

	checkWordpress := func() error {
		resp, err := nettestutil.HTTPGet("http://127.0.0.1:8080", 10, false)
		if err != nil {
			return err
		}
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		t.Logf("respBody=%q", respBody)
		if !strings.Contains(string(respBody), testutil.WordpressIndexHTMLSnippet) {
			return fmt.Errorf("respBody does not contain %q", testutil.WordpressIndexHTMLSnippet)
		}
		return nil
	}

	var wordpressWorking bool
	for i := 0; i < 30; i++ {
		t.Logf("(retry %d)", i)
		err := checkWordpress()
		if err == nil {
			wordpressWorking = true
			break
		}
		// NOTE: "<h1>Error establishing a database connection</h1>" is expected for the first few iterations
		t.Log(err)
		time.Sleep(3 * time.Second)
	}

	if !wordpressWorking {
		t.Fatal("wordpress is not working")
	}
	t.Log("wordpress seems functional")

	base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()
	base.Cmd("volume", "inspect", fmt.Sprintf("%s_db", projectName)).AssertFail()
	base.Cmd("network", "inspect", fmt.Sprintf("%s_default", projectName)).AssertFail()
}

func TestComposeUpBuild(t *testing.T) {
	defer func() {
		f := func(exe string, args ...string) {
			cmd := exec.Command(exe, args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stdout
			fmt.Printf("==== <%s> ====\n", cmd.Args)
			cmdErr := cmd.Run()
			fmt.Printf("==== </%s> (%v) ====\n", cmd.Args, cmdErr)
		}
		f("ps", "auxw")
		f("curl", "https://example.com")
		f("curl", "-I", "https://ghcr.io")
		f("ip", "a")
		if os.Geteuid() != 0 {
			g := func(exe string, args ...string) {
				f("containerd-rootless-setuptool.sh",
					append([]string{"nsenter", "--", exe}, args...)...)
			}
			g("nslookup", "ghcr.io", "10.0.2.3")
			g("nslookup", "ghcr.io", "8.8.8.8")
			g("curl", "https://example.com")
			g("curl", "-I", "https://ghcr.io")
			g("ip", "a")
			g("ip", "r")
			g("ping", "-c3", "10.0.2.2")
			g("ping", "-c3", "8.8.8.8")
		}
	}()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)

	const dockerComposeYAML = `
services:
  web:
    build: .
    ports:
    - 8080:80
`
	dockerfile := fmt.Sprintf(`FROM %s
COPY index.html /usr/share/nginx/html/index.html
`, testutil.NginxAlpineImage)
	indexHTML := t.Name()

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	comp.WriteFile("Dockerfile", dockerfile)
	comp.WriteFile("index.html", indexHTML)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d", "--build").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	resp, err := nettestutil.HTTPGet("http://127.0.0.1:8080", 50, false)
	assert.NilError(t, err)
	respBody, err := io.ReadAll(resp.Body)
	assert.NilError(t, err)
	t.Logf("respBody=%q", respBody)
	assert.Assert(t, strings.Contains(string(respBody), indexHTML))
}

func TestComposeUpMultiNet(t *testing.T) {
	base := testutil.NewBase(t)

	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
    networks:
      - net0
      - net1
      - net2
  svc1:
    image: %s
    networks:
      - net0
      - net1
  svc2:
    image: %s
    networks:
      - net2

networks:
  net0: {}
  net1: {}
  net2: {}
`, testutil.NginxAlpineImage, testutil.NginxAlpineImage, testutil.NginxAlpineImage)
	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	svc0 := fmt.Sprintf("%s_svc0_1", projectName)
	svc1 := fmt.Sprintf("%s_svc1_1", projectName)
	svc2 := fmt.Sprintf("%s_svc2_1", projectName)

	base.Cmd("exec", svc0, "ping", "-c", "1", "svc0").AssertOK()
	base.Cmd("exec", svc0, "ping", "-c", "1", "svc1").AssertOK()
	base.Cmd("exec", svc0, "ping", "-c", "1", "svc2").AssertOK()
	base.Cmd("exec", svc1, "ping", "-c", "1", "svc0").AssertOK()
	base.Cmd("exec", svc2, "ping", "-c", "1", "svc0").AssertOK()
	base.Cmd("exec", svc1, "ping", "-c", "1", "svc2").AssertFail()
}

func TestComposeUpOsEnvVar(t *testing.T) {
	base := testutil.NewBase(t)
	const containerName = "nginxAlpine"
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc1:
    image: %s
    container_name: %s
    ports:
      - ${ADDRESS:-127.0.0.1}:8080:80
`, testutil.NginxAlpineImage, containerName)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	base.Env = append(os.Environ(), "ADDRESS=0.0.0.0")

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	inspect := base.InspectContainer(containerName)
	inspect80TCP := (*inspect.NetworkSettings.Ports)["80/tcp"]
	expected := nat.PortBinding{
		HostIP:   "0.0.0.0",
		HostPort: "8080",
	}
	assert.Equal(base.T, expected, inspect80TCP[0])
}

func TestComposeUpDotEnvFile(t *testing.T) {
	base := testutil.NewBase(t)

	var dockerComposeYAML = `
version: '3.1'

services:
  svc3:
    image: ghcr.io/stargz-containers/nginx:$TAG
`

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	envFile := `TAG=1.19-alpine-org`
	comp.WriteFile(".env", envFile)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()
}

func TestComposeUpEnvFileNotFoundError(t *testing.T) {
	base := testutil.NewBase(t)

	var dockerComposeYAML = `
version: '3.1'

services:
  svc4:
    image: ghcr.io/stargz-containers/nginx:$TAG
`

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	envFile := `TAG=1.19-alpine-org`
	comp.WriteFile("envFile", envFile)

	//env-file is relative to the current working directory and not the project directory
	base.ComposeCmd("-f", comp.YAMLFullPath(), "--env-file", "envFile", "up", "-d").AssertFail()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()
}

func TestComposeUpWithScale(t *testing.T) {
	base := testutil.NewBase(t)

	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  test:
    image: %s
    command: "sleep infinity"
`, testutil.AlpineImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d", "--scale", "test=2").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps").AssertOutContains(fmt.Sprintf("%s_test_2", projectName))
}

func TestComposeIPAMConfig(t *testing.T) {
	base := testutil.NewBase(t)

	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  foo:
    image: %s
    command: "sleep infinity"

networks:
  default:
    ipam:
      config:
        - subnet: 10.1.100.0/24
`, testutil.AlpineImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	base.Cmd("inspect", "-f", `{{json .NetworkSettings.Networks }}`, projectName+"_foo_1").AssertOutContains("10.1.100.")
}
