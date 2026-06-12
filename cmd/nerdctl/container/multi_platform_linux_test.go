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

package container

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/platformutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/registry"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
)

// randomPort asks the registry helpers to acquire a free port automatically.
const randomPort = 0

// requireMultiPlatformExec skips the test when the host cannot execute
// linux/amd64, linux/arm64 and linux/arm/v7 images (e.g. no binfmt_misc).
var requireMultiPlatformExec = &test.Requirement{
	Check: func(_ test.Data, _ test.Helpers) (bool, string) {
		ok, err := platformutil.CanExecProbably("linux/amd64", "linux/arm64", "linux/arm/v7")
		if !ok {
			msg := "requires multi-platform exec support (linux/amd64, linux/arm64, linux/arm/v7)"
			if err != nil {
				msg += ": " + err.Error()
			}
			return false, msg
		}
		return true, ""
	},
}

// assertMultiPlatformRun runs uname -m inside image on each platform and
// asserts the expected machine type string.
func assertMultiPlatformRun(helpers test.Helpers, image string) {
	testCases := map[string]string{
		"amd64":        "x86_64",
		"arm64":        "aarch64",
		"arm":          "armv7l",
		"linux/arm":    "armv7l",
		"linux/arm/v7": "armv7l",
	}
	for plat, expectedUnameM := range testCases {
		helpers.T().Log(fmt.Sprintf("Testing platform %q (%q)", plat, expectedUnameM))
		helpers.Command("run", "--rm", "--platform="+plat, image, "uname", "-m").
			Run(&test.Expected{
				ExitCode: expect.ExitCodeSuccess,
				Output:   expect.Equals(expectedUnameM + "\n"),
			})
	}
}

func TestMultiPlatformRun(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = requireMultiPlatformExec

	testCase.Setup = func(_ test.Data, helpers test.Helpers) {
		assertMultiPlatformRun(helpers, testutil.AlpineImage)
	}

	testCase.Run(t)
}

func TestMultiPlatformBuildPush(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.All(
		// non-buildx `docker build` lacks multi-platform support; `docker push` lacks --platform
		require.Not(nerdtest.Docker),
		nerdtest.Build,
		requireMultiPlatformExec,
		nerdtest.Registry,
	)

	var reg *registry.Server

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		reg = nerdtest.RegistryWithNoAuth(data, helpers, randomPort, false)
		reg.Setup(data, helpers)
		imageName := fmt.Sprintf("localhost:%d/%s:latest", reg.Port, data.Identifier())
		data.Labels().Set("image", imageName)

		dockerfile := fmt.Sprintf("FROM %s\nRUN echo dummy\n", testutil.AlpineImage)
		buildCtx := data.Temp().Dir()
		data.Temp().Save(dockerfile, "Dockerfile")

		helpers.Ensure("build", "-t", imageName, "--platform=amd64,arm64,linux/arm/v7", buildCtx)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if img := data.Labels().Get("image"); img != "" {
			helpers.Anyhow("rmi", img)
		}
		helpers.Anyhow("builder", "prune", "--all", "--force")
		if reg != nil {
			reg.Cleanup(data, helpers)
		}
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		imageName := data.Labels().Get("image")
		assertMultiPlatformRun(helpers, imageName)
		return helpers.Command("push", "--platform=amd64,arm64,linux/arm/v7", imageName)
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, nil)

	testCase.Run(t)
}

func TestMultiPlatformBuildPushNoRun(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.All(
		// non-buildx `docker build` lacks multi-platform support; `docker push` lacks --platform
		require.Not(nerdtest.Docker),
		nerdtest.Build,
		requireMultiPlatformExec,
		nerdtest.Registry,
	)

	var reg *registry.Server

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		reg = nerdtest.RegistryWithNoAuth(data, helpers, randomPort, false)
		reg.Setup(data, helpers)
		imageName := fmt.Sprintf("localhost:%d/%s:latest", reg.Port, data.Identifier())
		data.Labels().Set("image", imageName)

		dockerfile := fmt.Sprintf("FROM %s\nCMD echo dummy\n", testutil.AlpineImage)
		buildCtx := data.Temp().Dir()
		data.Temp().Save(dockerfile, "Dockerfile")

		helpers.Ensure("build", "-t", imageName, "--platform=amd64,arm64,linux/arm/v7", buildCtx)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if img := data.Labels().Get("image"); img != "" {
			helpers.Anyhow("rmi", img)
		}
		helpers.Anyhow("builder", "prune", "--all", "--force")
		if reg != nil {
			reg.Cleanup(data, helpers)
		}
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		imageName := data.Labels().Get("image")
		assertMultiPlatformRun(helpers, imageName)
		return helpers.Command("push", "--platform=amd64,arm64,linux/arm/v7", imageName)
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, nil)

	testCase.Run(t)
}

func TestMultiPlatformPullPushAllPlatforms(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.All(
		require.Not(nerdtest.Docker),
		requireMultiPlatformExec,
		nerdtest.Registry,
	)

	var reg *registry.Server

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		reg = nerdtest.RegistryWithNoAuth(data, helpers, randomPort, false)
		reg.Setup(data, helpers)
		pushImageName := fmt.Sprintf("localhost:%d/%s:latest", reg.Port, data.Identifier())
		data.Labels().Set("image", pushImageName)
		helpers.Ensure("pull", "--quiet", "--all-platforms", testutil.AlpineImage)
		helpers.Ensure("tag", testutil.AlpineImage, pushImageName)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if img := data.Labels().Get("image"); img != "" {
			helpers.Anyhow("rmi", img)
		}
		if reg != nil {
			reg.Cleanup(data, helpers)
		}
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		pushImageName := data.Labels().Get("image")
		helpers.Ensure("push", "--all-platforms", pushImageName)
		assertMultiPlatformRun(helpers, pushImageName)
		return helpers.Command("inspect", "--type=image", pushImageName)
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, nil)

	testCase.Run(t)
}

func TestMultiPlatformComposeUpBuild(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.All(
		require.Not(nerdtest.Docker),
		nerdtest.Build,
		requireMultiPlatformExec,
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		dockerfile := fmt.Sprintf("FROM %s\nRUN uname -m > /usr/share/nginx/html/index.html\n", testutil.NginxAlpineImage)
		composeYAML := `
services:
  svc0:
    build: .
    platform: amd64
    ports:
    - 8080:80
  svc1:
    build: .
    platform: arm64
    ports:
    - 8081:80
  svc2:
    build: .
    platform: linux/arm/v7
    ports:
    - 8082:80
`
		buildCtx := data.Temp().Dir()
		composePath := data.Temp().Save(composeYAML, "compose.yaml")
		_ = buildCtx
		data.Temp().Save(dockerfile, "Dockerfile")
		data.Labels().Set("composePath", composePath)

		helpers.Ensure("compose", "-f", composePath, "up", "-d", "--build")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if cp := data.Labels().Get("composePath"); cp != "" {
			helpers.Anyhow("compose", "-f", cp, "down", "-v")
		}
		helpers.Anyhow("builder", "prune", "--all", "--force")
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		urlExpected := map[string]string{
			"http://127.0.0.1:8080": "x86_64",
			"http://127.0.0.1:8081": "aarch64",
			"http://127.0.0.1:8082": "armv7l",
		}
		for url, expected := range urlExpected {
			resp, err := nettestutil.HTTPGet(url, 5, false)
			if err != nil {
				helpers.T().Log(fmt.Sprintf("GET %s: %v", url, err))
				helpers.T().FailNow()
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				helpers.T().Log(fmt.Sprintf("reading body from %s: %v", url, err))
				helpers.T().FailNow()
			}
			if !strings.Contains(string(body), expected) {
				helpers.T().Log(fmt.Sprintf("expected %q in body from %s, got %q", expected, url, string(body)))
				helpers.T().Fail()
			}
		}
		return helpers.Command("compose", "-f", data.Labels().Get("composePath"), "ps")
	}

	testCase.Expected = test.Expects(expect.ExitCodeSuccess, nil, nil)

	testCase.Run(t)
}
