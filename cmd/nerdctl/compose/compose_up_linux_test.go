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
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/docker/go-connections/nat"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/portlock"
)

func TestComposeUp(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		hostPort, err := portlock.Acquire(0)
		if err != nil {
			helpers.T().Log(fmt.Sprintf("Failed to acquire port: %v", err))
			helpers.T().FailNow()
		}

		composeYAML := fmt.Sprintf(`
services:
  wordpress:
    image: %s
    restart: always
    ports:
      - %d:80
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
`, testutil.WordpressImage, hostPort, testutil.MariaDBImage)

		composePath := data.Temp().Save(composeYAML, "compose.yaml")

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		wordpressContainerName := serviceparser.DefaultContainerName(projectName, "wordpress", "1")
		dbContainerName := serviceparser.DefaultContainerName(projectName, "db", "1")

		data.Labels().Set("hostPort", strconv.Itoa(hostPort))
		data.Labels().Set("composeYAML", composePath)
		data.Labels().Set("projectName", projectName)
		data.Labels().Set("wordpressContainerName", wordpressContainerName)
		data.Labels().Set("dbContainerName", dbContainerName)

		helpers.Ensure("compose", "-f", composePath, "up", "-d")
		nerdtest.EnsureContainerStarted(helpers, wordpressContainerName)
		nerdtest.EnsureContainerStarted(helpers, dbContainerName)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "ps")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Output: expect.All(
				expect.Contains(data.Labels().Get("wordpressContainerName")),
				expect.Contains(data.Labels().Get("dbContainerName")),
			),
		}
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("composeYAML") != "" {
			helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
		}
		if p := data.Labels().Get("hostPort"); p != "" {
			if port, err := strconv.Atoi(p); err == nil {
				_ = portlock.Release(port)
			}
		}
		if projectName := data.Labels().Get("projectName"); projectName != "" {
			helpers.Command("volume", "inspect", fmt.Sprintf("%s_db", projectName)).Run(&test.Expected{ExitCode: 1})
			helpers.Command("network", "inspect", fmt.Sprintf("%s_default", projectName)).Run(&test.Expected{ExitCode: 1})
		}
	}

	testCase.Run(t)
}

func TestComposeUpBuild(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.Build

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		hostPort, err := portlock.Acquire(0)
		if err != nil {
			helpers.T().Log(fmt.Sprintf("Failed to acquire port: %v", err))
			helpers.T().FailNow()
		}

		composeYAML := fmt.Sprintf(`
services:
  web:
    build: .
    ports:
    - %d:80
`, hostPort)
		dockerfile := fmt.Sprintf(`FROM %s
COPY index.html /usr/share/nginx/html/index.html
`, testutil.NginxAlpineImage)
		indexHTML := data.Identifier("indexHTML")

		composePath := data.Temp().Save(composeYAML, "compose.yaml")
		data.Temp().Save(dockerfile, "Dockerfile")
		data.Temp().Save(indexHTML, "index.html")

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		data.Labels().Set("hostPort", strconv.Itoa(hostPort))
		data.Labels().Set("composeYAML", composePath)
		data.Labels().Set("indexHTML", data.Temp().Path("index.html"))

		helpers.Ensure("compose", "-f", composePath, "up", "-d", "--build")
		nerdtest.EnsureContainerStarted(helpers, serviceparser.DefaultContainerName(projectName, "web", "1"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "HTTP request to the web container",
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, t tig.T) {
						host := fmt.Sprintf("http://127.0.0.1:%s", data.Labels().Get("hostPort"))
						resp, err := nettestutil.HTTPGet(host, 5, false)
						assert.NilError(t, err)
						respBody, err := io.ReadAll(resp.Body)
						assert.NilError(t, err)
						t.Log(fmt.Sprintf("respBody=%q", respBody))
						assert.Assert(t, strings.Contains(string(respBody), data.Labels().Get("indexHTML")))
					},
				}
			},
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("composeYAML") != "" {
			helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
		}
		helpers.Anyhow("builder", "prune", "--all", "--force")
		if portStr := data.Labels().Get("hostPort"); portStr != "" {
			port, _ := strconv.Atoi(portStr)
			_ = portlock.Release(port)
		}
	}

	testCase.Run(t)
}

func TestComposeUpNetWithStaticIP(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.All(
		require.Not(nerdtest.Rootless),
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		staticIP := "10.4.255.254"
		subnet := "10.4.255.0/24"
		var composeYAML = fmt.Sprintf(`
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
        - subnet: %s
`, testutil.NginxAlpineImage, staticIP, subnet)

		composePath := data.Temp().Save(composeYAML, "compose.yaml")

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		containerName := serviceparser.DefaultContainerName(projectName, "svc0", "1")

		data.Labels().Set("staticIP", staticIP)
		data.Labels().Set("composeYAML", composePath)
		data.Labels().Set("containerName", containerName)

		helpers.Ensure("compose", "-f", composePath, "up", "-d")
		nerdtest.EnsureContainerStarted(helpers, containerName)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "static IP is assigned to container",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("inspect", data.Labels().Get("containerName"), "--format", "\"{{range .NetworkSettings.Networks}} {{.IPAddress}}{{end}}\"")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						assert.Assert(t, strings.Contains(stdout, data.Labels().Get("staticIP")))
					},
				}
			},
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("composeYAML") != "" {
			helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
		}
	}

	testCase.Run(t)
}

func TestComposeUpMultiNet(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		var composeYAML = fmt.Sprintf(`
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

		composePath := data.Temp().Save(composeYAML, "compose.yaml")

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		svc0 := serviceparser.DefaultContainerName(projectName, "svc0", "1")
		svc1 := serviceparser.DefaultContainerName(projectName, "svc1", "1")
		svc2 := serviceparser.DefaultContainerName(projectName, "svc2", "1")

		data.Labels().Set("composeYAML", composePath)
		data.Labels().Set("svc0", svc0)
		data.Labels().Set("svc1", svc1)
		data.Labels().Set("svc2", svc2)

		helpers.Ensure("compose", "-f", composePath, "up", "-d")
		nerdtest.EnsureContainerStarted(helpers, svc0)
		nerdtest.EnsureContainerStarted(helpers, svc1)
		nerdtest.EnsureContainerStarted(helpers, svc2)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "svc0 can ping itself",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Labels().Get("svc0"), "ping", "-c", "1", "svc0")
			},
			Expected: test.Expects(0, nil, nil),
		},
		{
			Description: "svc0 can ping svc1",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Labels().Get("svc0"), "ping", "-c", "1", "svc1")
			},
			Expected: test.Expects(0, nil, nil),
		},
		{
			Description: "svc0 can ping svc2",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Labels().Get("svc0"), "ping", "-c", "1", "svc2")
			},
			Expected: test.Expects(0, nil, nil),
		},
		{
			Description: "svc1 can ping svc0",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Labels().Get("svc1"), "ping", "-c", "1", "svc0")
			},
			Expected: test.Expects(0, nil, nil),
		},
		{
			Description: "svc2 can ping svc0",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Labels().Get("svc2"), "ping", "-c", "1", "svc0")
			},
			Expected: test.Expects(0, nil, nil),
		},
		{
			Description: "svc1 cannot ping svc2",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Labels().Get("svc1"), "ping", "-c", "1", "svc2")
			},
			Expected: test.Expects(1, nil, nil),
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("composeYAML") != "" {
			helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
		}
	}

	testCase.Run(t)
}

func TestComposeUpOsEnvVar(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Env = map[string]string{
		"ADDRESS": "0.0.0.0",
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		const containerName = "nginxAlpine"

		hostPort, err := portlock.Acquire(0)
		if err != nil {
			helpers.T().Log(fmt.Sprintf("Failed to acquire port: %v", err))
			helpers.T().FailNow()
		}

		var composeYAML = fmt.Sprintf(`
services:
  svc1:
    image: %s
    container_name: %s
    ports:
      - ${ADDRESS:-127.0.0.1}:%d:80
`, testutil.NginxAlpineImage, containerName, hostPort)

		composePath := data.Temp().Save(composeYAML, "compose.yaml")

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		data.Labels().Set("containerName", containerName)
		data.Labels().Set("hostPort", strconv.Itoa(hostPort))
		data.Labels().Set("composeYAML", composePath)

		helpers.Ensure("compose", "-f", composePath, "up", "-d")
		nerdtest.EnsureContainerStarted(helpers, containerName)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("container", "inspect", data.Labels().Get("containerName"))
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Output: expect.JSON([]dockercompat.Container{}, func(dc []dockercompat.Container, t tig.T) {
				assert.Equal(t, 1, len(dc), "unexpected number of containers")
				inspect80TCP := (*dc[0].NetworkSettings.Ports)["80/tcp"]
				assert.Assert(t, len(inspect80TCP) > 0, "no host bindings for 80/tcp")
				expected := nat.PortBinding{
					HostIP:   "0.0.0.0",
					HostPort: data.Labels().Get("hostPort"),
				}
				assert.Equal(t, expected, inspect80TCP[0])
			}),
		}
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("composeYAML") != "" {
			helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
		}
	}

	testCase.Run(t)
}

func TestComposeUpDotEnvFile(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		var composeYAML = `
services:
  svc3:
    image: ghcr.io/stargz-containers/nginx:$TAG
`

		composePath := data.Temp().Save(composeYAML, "compose.yaml")
		data.Temp().Save(`TAG=1.19-alpine-org`, ".env")

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		data.Labels().Set("composeYAML", composePath)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "up", "-d")
	}

	testCase.Expected = test.Expects(0, nil, nil)

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
	}

	testCase.Run(t)
}

func TestComposeUpEnvFileNotFoundError(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		var composeYAML = `
services:
  svc4:
    image: ghcr.io/stargz-containers/nginx:$TAG
`

		composePath := data.Temp().Save(composeYAML, "compose.yaml")
		data.Temp().Save(`TAG=1.19-alpine-org`, "envFile")

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		data.Labels().Set("composeYAML", composePath)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// env-file is relative to the current working directory and not the project directory
		return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "--env-file", "envFile", "up", "-d")
	}

	testCase.Expected = test.Expects(1, nil, nil)

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
	}

	testCase.Run(t)
}

func TestComposeUpWithScale(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		var composeYAML = fmt.Sprintf(`
services:
  test:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage)

		composePath := data.Temp().Save(composeYAML, "compose.yaml")

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		test1 := serviceparser.DefaultContainerName(projectName, "test", "1")
		test2 := serviceparser.DefaultContainerName(projectName, "test", "2")

		data.Labels().Set("composeYAML", composePath)
		data.Labels().Set("test1", test1)
		data.Labels().Set("test2", test2)

		helpers.Ensure("compose", "-f", composePath, "up", "-d", "--scale", "test=2")
		nerdtest.EnsureContainerStarted(helpers, test1)
		nerdtest.EnsureContainerStarted(helpers, test2)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "ps")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Output: expect.All(
				expect.Contains(data.Labels().Get("test1")),
				expect.Contains(data.Labels().Get("test2")),
			),
		}
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("composeYAML") != "" {
			helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
		}
	}

	testCase.Run(t)
}

func TestComposeIPAMConfig(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		var composeYAML = fmt.Sprintf(`
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

		composePath := data.Temp().Save(composeYAML, "compose.yaml")

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		fooContainer := serviceparser.DefaultContainerName(projectName, "foo", "1")

		data.Labels().Set("composeYAML", composePath)
		data.Labels().Set("fooContainer", fooContainer)

		helpers.Ensure("compose", "-f", composePath, "up", "-d")
		nerdtest.EnsureContainerStarted(helpers, fooContainer)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", "-f", "{{json .NetworkSettings.Networks }}", data.Labels().Get("fooContainer"))
	}

	testCase.Expected = test.Expects(0, nil, expect.Contains("10.1.100."))

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("composeYAML") != "" {
			helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
		}
	}

	testCase.Run(t)
}

func TestComposeUpRemoveOrphans(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
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

		composeOrphanPath := data.Temp().Save(dockerComposeYAMLOrphan, "compose-orphan.yaml")
		composeFullPath := data.Temp().Save(dockerComposeYAMLFull, "compose-full.yaml")

		projectName := data.Identifier("project")
		t.Logf("projectName=%q", projectName)

		testContainer := serviceparser.DefaultContainerName(projectName, "test", "1")
		orphanContainer := serviceparser.DefaultContainerName(projectName, "orphan", "1")

		data.Labels().Set("composeOrphanPath", composeOrphanPath)
		data.Labels().Set("composeFullPath", composeFullPath)
		data.Labels().Set("projectName", projectName)
		data.Labels().Set("orphanContainer", orphanContainer)

		helpers.Ensure("compose", "-p", projectName, "-f", composeFullPath, "up", "-d")
		helpers.Ensure("compose", "-p", projectName, "-f", composeOrphanPath, "up", "-d")
		nerdtest.EnsureContainerStarted(helpers, testContainer)
		nerdtest.EnsureContainerStarted(helpers, orphanContainer)

		helpers.Command("compose", "-p", projectName, "-f", composeFullPath, "ps").Run(
			&test.Expected{
				ExitCode: 0,
				Output:   expect.Contains(orphanContainer),
			},
		)
		helpers.Ensure("compose", "-p", projectName, "-f", composeOrphanPath, "up", "-d", "--remove-orphans")
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("compose", "-p", data.Labels().Get("projectName"), "-f", data.Labels().Get("composeFullPath"), "ps")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Output:   expect.DoesNotContain(data.Labels().Get("orphanContainer")),
		}
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("composeOrphanPath") != "" {
			helpers.Anyhow("compose", "-p", data.Labels().Get("projectName"), "-f", data.Labels().Get("composeOrphanPath"), "down", "-v")
		}
		if data.Labels().Get("composeFullPath") != "" {
			helpers.Anyhow("compose", "-p", data.Labels().Get("projectName"), "-f", data.Labels().Get("composeFullPath"), "down", "-v")
		}
	}

	testCase.Run(t)
}

func TestComposeUpIdempotent(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		composeYAML := fmt.Sprintf(`
services:
  test:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage)

		composePath := data.Temp().Save(composeYAML, "compose.yaml")

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		data.Labels().Set("composeYAML", composePath)

		helpers.Ensure("compose", "-f", composePath, "up", "-d")
		nerdtest.EnsureContainerStarted(helpers, serviceparser.DefaultContainerName(projectName, "test", "1"))
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "up", "-d")
	}

	testCase.Expected = test.Expects(0, nil, nil)

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("composeYAML") != "" {
			helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
		}
	}

	testCase.Run(t)
}

func TestComposeUpNoRecreateDependencies(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		var composeYAML = fmt.Sprintf(`
services:
  foo:
    image: %s
    command: "sleep infinity"
  bar:
    image: %s
    command: "sleep infinity"
    depends_on:
      - foo
`, testutil.CommonImage, testutil.CommonImage)

		composePath := data.Temp().Save(composeYAML, "compose.yaml")

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		fooContainer := serviceparser.DefaultContainerName(projectName, "foo", "1")
		barContainer := serviceparser.DefaultContainerName(projectName, "bar", "1")

		data.Labels().Set("composeYAML", composePath)
		data.Labels().Set("projectName", projectName)
		data.Labels().Set("fooContainer", fooContainer)
		data.Labels().Set("barContainer", barContainer)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "foo is not recreated when starting bar",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("compose", "-f", data.Labels().Get("composeYAML"), "up", "-d", "foo")
				nerdtest.EnsureContainerStarted(helpers, data.Labels().Get("fooContainer"))

				helpers.Command("inspect", data.Labels().Get("fooContainer"), "--format", "{{.Id}}").Run(
					&test.Expected{
						ExitCode: 0,
						Output: func(stdout string, t tig.T) {
							data.Labels().Set("fooContainerID", strings.TrimSpace(stdout))
						},
					},
				)

				// Bring up dependent service; ensure foo is not recreated (ID unchanged)
				helpers.Ensure("compose", "-f", data.Labels().Get("composeYAML"), "up", "-d", "bar")
				nerdtest.EnsureContainerStarted(helpers, data.Labels().Get("barContainer"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("inspect", data.Labels().Get("fooContainer"), "--format", "{{.Id}}")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: func(stdout string, t tig.T) {
						assert.Equal(t, strings.TrimSpace(stdout), data.Labels().Get("fooContainerID"))
					},
				}
			},
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("composeYAML") != "" {
			helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
		}
	}

	testCase.Run(t)
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
	testCase := nerdtest.Setup()

	testCase.Require = require.All(
		require.Not(nerdtest.Docker),
		nerdtest.Rootless,
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		testutil.RequireKernelVersion(t, ">= 5.9.0-0")
		testutil.RequireSystemService(t, "bypass4netnsd")

		hostPort, err := portlock.Acquire(0)
		if err != nil {
			helpers.T().Log(fmt.Sprintf("Failed to acquire port: %v", err))
			helpers.T().FailNow()
		}

		composeYAML := fmt.Sprintf(`
services:
  wordpress:
    image: %s
    restart: always
    ports:
      - %d:80
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
`, testutil.WordpressImage, hostPort, testutil.MariaDBImage)

		composePath := data.Temp().Save(composeYAML, "compose.yaml")
		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		data.Labels().Set("hostPort", strconv.Itoa(hostPort))
		data.Labels().Set("composeYAML", composePath)
		data.Labels().Set("projectName", projectName)

		helpers.Ensure("compose", "-f", composePath, "up", "-d")
		nerdtest.EnsureContainerStarted(helpers, serviceparser.DefaultContainerName(projectName, "wordpress", "1"))
		nerdtest.EnsureContainerStarted(helpers, serviceparser.DefaultContainerName(projectName, "db", "1"))

		helpers.Command("volume", "inspect", fmt.Sprintf("%s_db", projectName)).Run(&test.Expected{ExitCode: 0})
		helpers.Command("network", "inspect", fmt.Sprintf("%s_default", projectName)).Run(&test.Expected{ExitCode: 0})
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			Output: func(_ string, tt tig.T) {
				host := fmt.Sprintf("http://127.0.0.1:%s", data.Labels().Get("hostPort"))
				resp, err := nettestutil.HTTPGet(host, 5, false)
				assert.NilError(tt, err)
				body, err := io.ReadAll(resp.Body)
				assert.NilError(tt, err)
				_ = resp.Body.Close()
				assert.Assert(tt, strings.Contains(string(body), testutil.WordpressIndexHTMLSnippet))
				t.Log("wordpress seems functional")
			},
		}
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Labels().Get("composeYAML") != "" {
			helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
		}
		if p := data.Labels().Get("hostPort"); p != "" {
			if port, err := strconv.Atoi(p); err == nil {
				_ = portlock.Release(port)
			}
		}

		if projectName := data.Labels().Get("projectName"); projectName != "" {
			helpers.Command("volume", "inspect", fmt.Sprintf("%s_db", projectName)).Run(&test.Expected{ExitCode: 1})
			helpers.Command("network", "inspect", fmt.Sprintf("%s_default", projectName)).Run(&test.Expected{ExitCode: 1})
		}
	}

	testCase.Run(t)
}

func TestComposeUpProfile(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		serviceRegular := data.Identifier("regular")
		serviceProfiled := data.Identifier("profiled")

		envFilePath := data.Temp().Save(`TEST_ENV_INJECTION=WORKS\n`, "env.common")

		composeYAML := fmt.Sprintf(`
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

		composePath := data.Temp().Save(composeYAML, "compose.yaml")

		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		data.Labels().Set("serviceRegular", serviceRegular)
		data.Labels().Set("serviceProfiled", serviceProfiled)
		data.Labels().Set("composeYAML", composePath)
		data.Labels().Set("regularContainer", serviceparser.DefaultContainerName(projectName, serviceRegular, "1"))
		data.Labels().Set("profiledContainer", serviceparser.DefaultContainerName(projectName, serviceProfiled, "1"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "with profile",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("compose", "-f", data.Labels().Get("composeYAML"), "--profile", "test-profile", "up", "-d")
				nerdtest.EnsureContainerStarted(helpers, data.Labels().Get("regularContainer"))
				nerdtest.EnsureContainerStarted(helpers, data.Labels().Get("profiledContainer"))

				helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "exec", data.Labels().Get("serviceProfiled"), "env").
					Run(&test.Expected{
						ExitCode: 0,
						Output:   expect.Contains("TEST_ENV_INJECTION=WORKS"),
					})
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "-a", "--format={{.Names}}")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(
						expect.Contains(data.Labels().Get("serviceRegular")),
						expect.Contains(data.Labels().Get("serviceProfiled")),
					),
				}
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "--profile", "test-profile", "down", "-v")
			},
		},
		{
			Description: "profiled not started without profile flag",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("compose", "-f", data.Labels().Get("composeYAML"), "up", "-d")
				nerdtest.EnsureContainerStarted(helpers, data.Labels().Get("regularContainer"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "-a", "--format={{.Names}}")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(
						expect.Contains(data.Labels().Get("serviceRegular")),
						expect.DoesNotContain(data.Labels().Get("serviceProfiled")),
					),
				}
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
			},
		},
	}

	testCase.Run(t)
}

func TestComposeUpAbortOnContainerExit(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		serviceRegular := data.Identifier("regular")
		serviceProfiled := data.Identifier("exited")
		composeYAML := fmt.Sprintf(`
services:
  %s:
    image: %s
  %s:
    image: %s
    entrypoint: /bin/sh -c "exit 1"
`, serviceRegular, testutil.NginxAlpineImage, serviceProfiled, testutil.BusyboxImage)

		composePath := data.Temp().Save(composeYAML, "compose.yaml")
		projectName := filepath.Base(filepath.Dir(composePath))
		t.Logf("projectName=%q", projectName)

		data.Labels().Set("serviceRegular", serviceRegular)
		data.Labels().Set("serviceProfiled", serviceProfiled)
		data.Labels().Set("composeYAML", composePath)
		data.Labels().Set("regularContainer", serviceparser.DefaultContainerName(projectName, serviceRegular, "1"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "abort on container exit",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "up", "--abort-on-container-exit").Run(
					&test.Expected{
						ExitCode: 1,
					},
				)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "-a", "--format={{.Names}}", "--filter", "status=exited")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(
						expect.Contains(data.Labels().Get("serviceRegular")),
						expect.Contains(data.Labels().Get("serviceProfiled")),
					),
				}
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
			},
		},
		{
			Description: "no abort flag keeps other services running",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("compose", "-f", data.Labels().Get("composeYAML"), "up", "-d")
				nerdtest.EnsureContainerStarted(helpers, data.Labels().Get("regularContainer"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("ps", "-a", "--format={{.Names}}", "--filter", "status=exited")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output: expect.All(
						expect.DoesNotContain(data.Labels().Get("serviceRegular")),
						expect.Contains(data.Labels().Get("serviceProfiled")),
					),
				}
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down", "-v")
			},
		},
		// in this sub-test we are ensuring that flags '-d' and '--abort-on-container-exit' cannot be ran together
		{
			Description: "flag -d incompatible with --abort-on-container-exit",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "up", "-d", "--abort-on-container-exit")
			},
			Expected: test.Expects(1, nil, nil),
		},
	}

	testCase.Run(t)
}

func TestComposeUpPull(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.NoParallel = true
	testCase.Require = nerdtest.Private

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		composeYAML := fmt.Sprintf(`
services:
  test:
    image: %s
    command: sh -euxc "echo hi"
`, testutil.CommonImage)

		composePath := data.Temp().Save(composeYAML, "compose.yaml")

		data.Labels().Set("composeYAML", composePath)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "pull=missing",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("rmi", "-f", testutil.CommonImage)
				helpers.Command("images").Run(
					&test.Expected{
						ExitCode: 0,
						Output:   expect.DoesNotContain(testutil.CommonImage),
					},
				)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "up", "--pull", "missing")
			},
			Expected: test.Expects(0, nil, expect.Contains("hi")),
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down")
			},
		},
		{
			Description: "pull=always",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("rmi", "-f", testutil.CommonImage)
				helpers.Command("images").Run(
					&test.Expected{
						ExitCode: 0,
						Output:   expect.DoesNotContain(testutil.CommonImage),
					},
				)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "up", "--pull", "always")
			},
			Expected: test.Expects(0, nil, expect.Contains("hi")),
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down")
			},
		},
		{
			Description: "pull=never, no pull",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("rmi", "-f", testutil.CommonImage)
				helpers.Command("images").Run(
					&test.Expected{
						ExitCode: 0,
						Output:   expect.DoesNotContain(testutil.CommonImage),
					},
				)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "up", "--pull", "never")
			},
			Expected: test.Expects(1, nil, nil),
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down")
			},
		},
	}

	testCase.Run(t)
}

func TestComposeUpServicePullPolicy(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.Private

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		var composeYAML = fmt.Sprintf(`
services:
  test:
    image: %s
    command: sh -euxc "echo hi"
    pull_policy: "never"
`, testutil.CommonImage)

		composePath := data.Temp().Save(composeYAML, "compose.yaml")

		data.Labels().Set("composeYAML", composePath)

		helpers.Ensure("rmi", "-f", testutil.CommonImage)
		helpers.Command("images").Run(
			&test.Expected{
				ExitCode: 0,
				Output:   expect.DoesNotContain(testutil.CommonImage),
			},
		)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("compose", "-f", data.Labels().Get("composeYAML"), "up")
	}

	testCase.Expected = test.Expects(1, nil, nil)

	testCase.Run(t)
}

func TestComposeTmpfsVolume(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		containerName := data.Identifier("tmpfs")
		composeYAML := fmt.Sprintf(`
services:
  tmpfs:
    container_name: %s
    image: %s
    command: sleep infinity
    volumes:
      - type: tmpfs
        target: /target-rw
        tmpfs:
          size: 64m
      - type: tmpfs
        target: /target-ro
        read_only: true
        tmpfs:
          size: 64m
          mode: 0o1770
`, containerName, testutil.CommonImage)

		composeYAMLPath := data.Temp().Save(composeYAML, "compose.yaml")

		helpers.Ensure("compose", "-f", composeYAMLPath, "up", "-d")
		nerdtest.EnsureContainerStarted(helpers, containerName)

		data.Labels().Set("composeYAML", composeYAMLPath)
		data.Labels().Set("containerName", containerName)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "rw tmpfs mount",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Labels().Get("containerName"), "grep", "/target-rw", "/proc/mounts")
			},
			Expected: test.Expects(0, nil,
				expect.All(
					expect.Contains("/target-rw"),
					expect.Contains("rw"),
					expect.Contains("size=65536k"),
				),
			),
		},
		{
			Description: "ro tmpfs mount with mode",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("exec", data.Labels().Get("containerName"), "grep", "/target-ro", "/proc/mounts")
			},
			Expected: test.Expects(0, nil,
				expect.All(
					expect.Contains("/target-ro"),
					expect.Contains("ro"),
					expect.Contains("size=65536k"),
					expect.Contains("mode=1770"),
				),
			),
		},
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Labels().Get("composeYAML"), "down")
	}

	testCase.Run(t)
}

func TestComposeUpWithIPv6(t *testing.T) {
	base := testutil.NewBaseWithIPv6Compatible(t)

	subnet := "2001:aaa::/64"
	var dockerComposeYAML = fmt.Sprintf(`
services:
  svc0:
    image: %s
    networks:
    - net0
networks:
  net0:
    enable_ipv6: true
    ipam:
      config:
      - subnet: %s`, testutil.CommonImage, subnet)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)
	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	inspectCmd := base.Cmd("network", "inspect", projectName+"_net0", "--format", "\"{{range .IPAM.Config}}{{.Subnet}} {{end}}\"")
	result := inspectCmd.Run()
	stdoutContent := result.Stdout() + result.Stderr()
	assert.Assert(inspectCmd.Base.T, result.ExitCode == 0, stdoutContent)

	if !strings.Contains(stdoutContent, subnet) {
		log.L.Errorf("test failed, the actual subnets are %s", stdoutContent)
		t.Fail()
		return
	}
}

func TestComposeUpWithIPv6Disabled(t *testing.T) {
	base := testutil.NewBaseWithIPv6Compatible(t)

	subnet := "2001:aab::/64"
	var dockerComposeYAML = fmt.Sprintf(`
services:
  svc0:
    image: %s
    networks:
    - net0
networks:
  net0:
    enable_ipv6: false
    ipam:
      config:
      - subnet: %s`, testutil.CommonImage, subnet)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)
	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	inspectCmd := base.Cmd("network", "inspect", projectName+"_net0", "--format", "\"{{range .IPAM.Config}}{{.Subnet}} {{end}}\"")
	result := inspectCmd.Run()
	stdoutContent := result.Stdout() + result.Stderr()
	assert.Assert(inspectCmd.Base.T, result.ExitCode == 0, stdoutContent)

	if strings.Contains(stdoutContent, subnet) {
		log.L.Errorf("test failed, the actual subnets are %s", stdoutContent)
		t.Fail()
		return
	}
}
