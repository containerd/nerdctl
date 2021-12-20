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

	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/containerd/nerdctl/pkg/testutil/nettestutil"
	"github.com/containerd/nerdctl/pkg/testutil/testregistry"
	"gotest.tools/v3/assert"
)

func testMultiPlatformRun(base *testutil.Base, alpineImage string) {
	t := base.T
	testutil.RequireExecPlatform(t, "linux/amd64", "linux/arm64", "linux/arm/v7")
	testCases := map[string]string{
		"amd64":        "x86_64",
		"arm64":        "aarch64",
		"arm":          "armv7l",
		"linux/arm":    "armv7l",
		"linux/arm/v7": "armv7l",
	}
	for plat, expectedUnameM := range testCases {
		t.Logf("Testing %q (%q)", plat, expectedUnameM)
		cmd := base.Cmd("run", "--rm", "--platform="+plat, alpineImage, "uname", "-m")
		cmd.AssertOutExactly(expectedUnameM + "\n")
	}
}

func TestMultiPlatformRun(t *testing.T) {
	base := testutil.NewBase(t)
	testMultiPlatformRun(base, testutil.AlpineImage)
}

func TestMultiPlatformBuildPush(t *testing.T) {
	testutil.DockerIncompatible(t) // non-buildx version of `docker build` lacks multi-platform. Also, `docker push` lacks --platform.
	testutil.RequiresBuild(t)
	testutil.RequireExecPlatform(t, "linux/amd64", "linux/arm64", "linux/arm/v7")
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	reg := testregistry.NewPlainHTTP(base)
	defer reg.Cleanup()

	imageName := fmt.Sprintf("localhost:%d/%s:latest", reg.ListenPort, tID)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
RUN echo dummy
	`, testutil.AlpineImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, "--platform=amd64,arm64,linux/arm/v7", buildCtx).AssertOK()
	testMultiPlatformRun(base, imageName)
	base.Cmd("push", "--platform=amd64,arm64,linux/arm/v7", imageName).AssertOK()
}

func TestMultiPlatformPullPushAllPlatforms(t *testing.T) {
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	reg := testregistry.NewPlainHTTP(base)
	defer reg.Cleanup()

	pushImageName := fmt.Sprintf("localhost:%d/%s:latest", reg.ListenPort, tID)
	defer base.Cmd("rmi", pushImageName).Run()

	base.Cmd("pull", "--all-platforms", testutil.AlpineImage).AssertOK()
	base.Cmd("tag", testutil.AlpineImage, pushImageName).AssertOK()
	base.Cmd("push", "--all-platforms", pushImageName).AssertOK()
	testMultiPlatformRun(base, pushImageName)
}

func TestMultiPlatformComposeUpBuild(t *testing.T) {
	testutil.DockerIncompatible(t)
	testutil.RequiresBuild(t)
	testutil.RequireExecPlatform(t, "linux/amd64", "linux/arm64", "linux/arm/v7")
	base := testutil.NewBase(t)

	const dockerComposeYAML = `
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
	dockerfile := fmt.Sprintf(`FROM %s
RUN uname -m > /usr/share/nginx/html/index.html
`, testutil.NginxAlpineImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	comp.WriteFile("Dockerfile", dockerfile)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d", "--build").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	testCases := map[string]string{
		"http://127.0.0.1:8080": "x86_64",
		"http://127.0.0.1:8081": "aarch64",
		"http://127.0.0.1:8082": "armv7l",
	}

	for testURL, expectedIndexHTML := range testCases {
		resp, err := nettestutil.HTTPGet(testURL, 50, false)
		assert.NilError(t, err)
		respBody, err := io.ReadAll(resp.Body)
		assert.NilError(t, err)
		t.Logf("respBody=%q", respBody)
		assert.Assert(t, strings.Contains(string(respBody), expectedIndexHTML))
	}
}
