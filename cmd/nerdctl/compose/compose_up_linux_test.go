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
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"

	"github.com/containerd/log"
	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
)

func TestComposeUp(t *testing.T) {
	base := testutil.NewBase(t)
	helpers.ComposeUp(t, base, fmt.Sprintf(`
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

func TestComposeUpBuild(t *testing.T) {
	testutil.RequiresBuild(t)
	testutil.RegisterBuildCacheCleanup(t)
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
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	comp.WriteFile("Dockerfile", dockerfile)
	comp.WriteFile("index.html", indexHTML)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d", "--build").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	resp, err := nettestutil.HTTPGet("http://127.0.0.1:8080", 5, false)
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
		log.L.Errorf("test failed, the actual container ip is %s", stdoutContent)
		t.Fail()
		return
	}
}

func TestComposeUpMultiNet(t *testing.T) {
	base := testutil.NewBase(t)

	var dockerComposeYAML = fmt.Sprintf(`
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

	base.Env = append(base.Env, "ADDRESS=0.0.0.0")

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
services:
  test:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage)

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
services:
  foo:
    image: %s
    command: "sleep infinity"

networks:
  default:
    ipam:
      config:
        - subnet: 10.1.100.0/24
`, testutil.CommonImage)

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
services:
  test:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage)

		dockerComposeYAMLFull = fmt.Sprintf(`
%s
  orphan:
    image: %s
    command: "sleep infinity"
`, dockerComposeYAMLOrphan, testutil.CommonImage)
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
services:
  test:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage)

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
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		var dockerComposeYaml1 = fmt.Sprintf(`
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
`, data.Identifier("con-1"), testutil.NginxAlpineImage, data.Identifier("con-1"), data.Identifier("network"), data.Identifier("network"))
		var dockerComposeYaml2 = fmt.Sprintf(`
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
`, data.Identifier("con-2"), testutil.NginxAlpineImage, data.Identifier("con-2"), data.Identifier("network"), data.Identifier("network"))
		tmp := data.Temp()

		tmp.Save(dockerComposeYaml1, "project-1", "compose.yaml")
		tmp.Save(dockerComposeYaml2, "project-2", "compose.yaml")

		helpers.Ensure("network", "create", data.Identifier("network"))
		helpers.Ensure("compose", "-f", tmp.Path("project-1", "compose.yaml"), "up", "-d")
		helpers.Ensure("compose", "-f", tmp.Path("project-2", "compose.yaml"), "up", "-d")
		helpers.Ensure("compose", "-f", tmp.Path("project-2", "compose.yaml"), "down", "-v")
		helpers.Ensure("compose", "-f", tmp.Path("project-2", "compose.yaml"), "up", "-d")
		nerdtest.EnsureContainerStarted(helpers, data.Identifier("con-2"))
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		helpers.Ensure("exec", data.Identifier("con-1"), "cat", "/etc/hosts")
		return helpers.Command("exec", data.Identifier("con-1"), "wget", "-qO-", "http://"+data.Identifier("con-2"))
	}

	testCase.Expected = test.Expects(0, nil, expect.Contains(testutil.NginxAlpineIndexHTMLSnippet))

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("project-1", "compose.yaml"), "down", "-v")
		helpers.Anyhow("compose", "-f", data.Temp().Path("project-2", "compose.yaml"), "down", "-v")
		helpers.Anyhow("network", "rm", data.Identifier("network"))
	}

	testCase.Run(t)
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
	helpers.ComposeUp(t, base, fmt.Sprintf(`
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
    annotations:
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
    annotations:
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

	// write the env.common file to tmpdir
	tmpDir := t.TempDir()
	envFilePath := fmt.Sprintf("%s/env.common", tmpDir)
	err := os.WriteFile(envFilePath, []byte("TEST_ENV_INJECTION=WORKS\n"), 0644)
	assert.NilError(t, err)

	dockerComposeYAML := fmt.Sprintf(`
services:
  %s:
    image: %[3]s

  %[2]s:
    image: %[3]s
    profiles:
      - test-profile
    env_file:
      - %[4]s
`, serviceRegular, serviceProfiled, testutil.NginxAlpineImage, envFilePath)

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

	execCmd := base.ComposeCmd("-f", comp1.YAMLFullPath(), "exec", serviceProfiled, "env")
	execCmd.AssertOutContains("TEST_ENV_INJECTION=WORKS")

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

func TestComposeUpAbortOnContainerExit(t *testing.T) {
	base := testutil.NewBase(t)
	serviceRegular := "regular"
	serviceProfiled := "exited"
	dockerComposeYAML := fmt.Sprintf(`
services:
  %s:
    image: %s
  %s:
    image: %s
    entrypoint: /bin/sh -c "exit 1"
`, serviceRegular, testutil.NginxAlpineImage, serviceProfiled, testutil.BusyboxImage)
	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	// here we run 'compose up --abort-on-container-exit' command
	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "--abort-on-container-exit").AssertExitCode(1)
	time.Sleep(3 * time.Second)
	psCmd := base.Cmd("ps", "-a", "--format={{.Names}}", "--filter", "status=exited")

	psCmd.AssertOutContains(serviceRegular)
	psCmd.AssertOutContains(serviceProfiled)
	base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	// this time we run 'compose up' command without --abort-on-container-exit flag
	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	time.Sleep(3 * time.Second)
	psCmd = base.Cmd("ps", "-a", "--format={{.Names}}", "--filter", "status=exited")

	// this time the regular service should not be listed in the output
	psCmd.AssertOutNotContains(serviceRegular)
	psCmd.AssertOutContains(serviceProfiled)
	base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	// in this sub-test we are ensuring that flags '-d' and '--abort-on-container-exit' cannot be ran together
	c := base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d", "--abort-on-container-exit")
	expected := icmd.Expected{
		ExitCode: 1,
	}
	c.Assert(expected)
}

func TestComposeUpPull(t *testing.T) {
	base := testutil.NewBase(t)

	var dockerComposeYAML = fmt.Sprintf(`
services:
  test:
    image: %s
    command: sh -euxc "echo hi"
`, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	// Cases where pull is required
	for _, pull := range []string{"missing", "always"} {
		t.Run(fmt.Sprintf("pull=%s", pull), func(t *testing.T) {
			base.Cmd("rmi", "-f", testutil.CommonImage).Run()
			base.Cmd("images").AssertOutNotContains(testutil.CommonImage)
			t.Cleanup(func() {
				base.ComposeCmd("-f", comp.YAMLFullPath(), "down").AssertOK()
			})
			base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "--pull", pull).AssertOutContains("hi")
		})
	}

	t.Run("pull=never, no pull", func(t *testing.T) {
		base.Cmd("rmi", "-f", testutil.CommonImage).Run()
		base.Cmd("images").AssertOutNotContains(testutil.CommonImage)
		t.Cleanup(func() {
			base.ComposeCmd("-f", comp.YAMLFullPath(), "down").AssertOK()
		})
		base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "--pull", "never").AssertExitCode(1)
	})
}

func TestComposeUpServicePullPolicy(t *testing.T) {
	base := testutil.NewBase(t)

	var dockerComposeYAML = fmt.Sprintf(`
services:
  test:
    image: %s
    command: sh -euxc "echo hi"
    pull_policy: "never"
`, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	base.Cmd("rmi", "-f", testutil.CommonImage).Run()
	base.Cmd("images").AssertOutNotContains(testutil.CommonImage)
	base.ComposeCmd("-f", comp.YAMLFullPath(), "up").AssertExitCode(1)
}
