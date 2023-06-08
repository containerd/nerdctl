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
	"strings"
	"testing"
	"time"

	"github.com/containerd/nerdctl/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/docker/go-connections/nat"
	"github.com/sirupsen/logrus"

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

func testComposeUp(t *testing.T, base *testutil.Base, dockerComposeYAML string, opts ...string) {
	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd(append(append([]string{"-f", comp.YAMLFullPath()}, opts...), "up", "-d")...).AssertOK()
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
		if !strings.Contains(string(respBody), testutil.WordpressIndexHTMLSnippet) {
			t.Logf("respBody=%q", respBody)
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
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()

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
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

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

func TestComposeUpNetWithStaticIP(t *testing.T) {
	if rootlessutil.IsRootless() {
		t.Skip("Static IP assignment is not supported rootless mode yet.")
	}
	base := testutil.NewBase(t)
	staticIP := "172.20.0.12"
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
    networks:
      net0:
        ipv4_address: %s

networks:
  net0:
    ipam:
      config:
        - subnet: 172.20.0.0/24
`, testutil.NginxAlpineImage, staticIP)
	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)
	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	svc0 := serviceparser.DefaultContainerName(projectName, "svc0", "1")
	inspectCmd := base.Cmd("inspect", svc0, "--format", "\"{{range .NetworkSettings.Networks}} {{.IPAddress}}{{end}}\"")
	result := inspectCmd.Run()
	stdoutContent := result.Stdout() + result.Stderr()
	assert.Assert(inspectCmd.Base.T, result.ExitCode == 0, stdoutContent)
	if !strings.Contains(stdoutContent, staticIP) {
		logrus.Errorf("test failed, the actual container ip is %s", stdoutContent)
		t.Fail()
		return
	}
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

	svc0 := serviceparser.DefaultContainerName(projectName, "svc0", "1")
	svc1 := serviceparser.DefaultContainerName(projectName, "svc1", "1")
	svc2 := serviceparser.DefaultContainerName(projectName, "svc2", "1")

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
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

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
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

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
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

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

	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps").AssertOutContains(serviceparser.DefaultContainerName(projectName, "test", "2"))
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

	base.Cmd("inspect", "-f", `{{json .NetworkSettings.Networks }}`, serviceparser.DefaultContainerName(projectName, "foo", "1")).AssertOutContains("10.1.100.")
}

func TestComposeUpRemoveOrphans(t *testing.T) {
	base := testutil.NewBase(t)

	var (
		dockerComposeYAMLOrphan = fmt.Sprintf(`
version: '3.1'

services:
  test:
    image: %s
    command: "sleep infinity"
`, testutil.AlpineImage)

		dockerComposeYAMLFull = fmt.Sprintf(`
%s
  orphan:
    image: %s
    command: "sleep infinity"
`, dockerComposeYAMLOrphan, testutil.AlpineImage)
	)

	compOrphan := testutil.NewComposeDir(t, dockerComposeYAMLOrphan)
	defer compOrphan.CleanUp()
	compFull := testutil.NewComposeDir(t, dockerComposeYAMLFull)
	defer compFull.CleanUp()

	projectName := fmt.Sprintf("nerdctl-compose-test-%d", time.Now().Unix())
	t.Logf("projectName=%q", projectName)

	orphanContainer := serviceparser.DefaultContainerName(projectName, "orphan", "1")

	base.ComposeCmd("-p", projectName, "-f", compFull.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-p", projectName, "-f", compFull.YAMLFullPath(), "down", "-v").Run()
	base.ComposeCmd("-p", projectName, "-f", compOrphan.YAMLFullPath(), "up", "-d").AssertOK()
	base.ComposeCmd("-p", projectName, "-f", compFull.YAMLFullPath(), "ps").AssertOutContains(orphanContainer)
	base.ComposeCmd("-p", projectName, "-f", compOrphan.YAMLFullPath(), "up", "-d", "--remove-orphans").AssertOK()
	base.ComposeCmd("-p", projectName, "-f", compFull.YAMLFullPath(), "ps").AssertOutNotContains(orphanContainer)
}

func TestComposeUpIdempotent(t *testing.T) {
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

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "down").AssertOK()
}

func TestComposeUpWithExternalNetwork(t *testing.T) {
	containerName1 := testutil.Identifier(t) + "-1"
	containerName2 := testutil.Identifier(t) + "-2"
	networkName := testutil.Identifier(t) + "-network"
	var dockerComposeYaml1 = fmt.Sprintf(`
version: "3"
services:
  %s:
    image: %s
    container_name: %s
    networks:
      %s:
        aliases:
          - nginx-1
networks:
  %s:
    external: true
`, containerName1, testutil.NginxAlpineImage, containerName1, networkName, networkName)
	var dockerComposeYaml2 = fmt.Sprintf(`
version: "3"
services:
  %s:
    image: %s
    container_name: %s
    networks:
      %s:
        aliases:
          - nginx-2
networks:
  %s:
    external: true
`, containerName2, testutil.NginxAlpineImage, containerName2, networkName, networkName)
	comp1 := testutil.NewComposeDir(t, dockerComposeYaml1)
	defer comp1.CleanUp()
	comp2 := testutil.NewComposeDir(t, dockerComposeYaml2)
	defer comp2.CleanUp()
	base := testutil.NewBase(t)
	// Create the test network
	base.Cmd("network", "create", networkName).AssertOK()
	defer base.Cmd("network", "rm", networkName).Run()
	// Run the first compose
	base.ComposeCmd("-f", comp1.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp1.YAMLFullPath(), "down", "-v").Run()
	// Run the second compose
	base.ComposeCmd("-f", comp2.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp2.YAMLFullPath(), "down", "-v").Run()
	// Down the second compose
	base.ComposeCmd("-f", comp2.YAMLFullPath(), "down", "-v").AssertOK()
	// Run the second compose again
	base.ComposeCmd("-f", comp2.YAMLFullPath(), "up", "-d").AssertOK()
	base.Cmd("exec", containerName1, "wget", "-qO-", "http://"+containerName2).AssertOutContains(testutil.NginxAlpineIndexHTMLSnippet)
}

func TestComposeUpWithBypass4netns(t *testing.T) {
	// docker does not support bypass4netns mode
	testutil.DockerIncompatible(t)
	if !rootlessutil.IsRootless() {
		t.Skip("test needs rootless")
	}
	testutil.RequireKernelVersion(t, ">= 5.9.0-0")
	testutil.RequireSystemService(t, "bypass4netnsd")
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
    labels:
      - nerdctl/bypass4netns=1

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
    labels:
      - nerdctl/bypass4netns=1

volumes:
  wordpress:
  db:
`, testutil.WordpressImage, testutil.MariaDBImage))
}

func TestComposeUpProfile(t *testing.T) {
	base := testutil.NewBase(t)
	serviceRegular := testutil.Identifier(t) + "-regular"
	serviceProfiled := testutil.Identifier(t) + "-profiled"

	dockerComposeYAML := fmt.Sprintf(`
services:
  %s:
    image: %[3]s

  %[2]s:
    image: %[3]s
    profiles:
      - test-profile
`, serviceRegular, serviceProfiled, testutil.NginxAlpineImage)

	// * Test with profile
	//   Should run both the services:
	//     - matching active profile
	//     - one without profile
	comp1 := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp1.CleanUp()
	base.ComposeCmd("-f", comp1.YAMLFullPath(), "--profile", "test-profile", "up", "-d").AssertOK()

	psCmd := base.Cmd("ps", "-a", "--format={{.Names}}")
	psCmd.AssertOutContains(serviceRegular)
	psCmd.AssertOutContains(serviceProfiled)
	base.ComposeCmd("-f", comp1.YAMLFullPath(), "--profile", "test-profile", "down", "-v").AssertOK()

	// * Test without profile
	//   Should run:
	//     - service without profile
	comp2 := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp2.CleanUp()
	base.ComposeCmd("-f", comp2.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp2.YAMLFullPath(), "down", "-v").AssertOK()

	psCmd = base.Cmd("ps", "-a", "--format={{.Names}}")
	psCmd.AssertOutContains(serviceRegular)
	psCmd.AssertOutNotContains(serviceProfiled)
}
