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

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestComposeCreate(t *testing.T) {
	base := testutil.NewBase(t)
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
`, testutil.AlpineImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	// 1.1 `compose create` should create service container (in `created` status)
	base.ComposeCmd("-f", comp.YAMLFullPath(), "create").AssertOK()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "svc0", "-a").AssertOutContainsAny("Created", "created")
	// 1.2 created container can be started by `compose start`
	base.ComposeCmd("-f", comp.YAMLFullPath(), "start").AssertOK()
}

func TestComposeCreateDependency(t *testing.T) {
	// docker-compose v1 depecreated this command
	// docker-compose v2 reimplemented this command
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
    depends_on:
    - "svc1"
  svc1:
    image: %s
`, testutil.CommonImage, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	// `compose create` should create containers for both services and their dependencies
	base.ComposeCmd("-f", comp.YAMLFullPath(), "create", "svc0").AssertOK()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "svc0", "-a").AssertOutContainsAny("Created", "created")
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "svc1", "-a").AssertOutContainsAny("Created", "created")
}

func TestComposeCreatePull(t *testing.T) {
	// docker-compose v1 depecreated this command
	// docker-compose v2 reimplemented this command
	testutil.DockerIncompatible(t)

	base := testutil.NewBase(t)
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
`, testutil.AlpineImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	// `compose create --pull never` should fail: no such image
	base.Cmd("rmi", "-f", testutil.AlpineImage).Run()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "create", "--pull", "never").AssertFail()
	// `compose create --pull missing(default)|always` should succeed: image is pulled and container is created
	base.Cmd("rmi", "-f", testutil.AlpineImage).Run()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "create").AssertOK()
	base.Cmd("rmi", "-f", testutil.AlpineImage).Run()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "create", "--pull", "always").AssertOK()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "svc0", "-a").AssertOutContainsAny("Created", "created")
}

func TestComposeCreateBuild(t *testing.T) {
	// docker-compose v1 depecreated this command
	// docker-compose v2 reimplemented this command
	testutil.DockerIncompatible(t)

	const imageSvc0 = "composebuild_svc0"

	dockerComposeYAML := fmt.Sprintf(`
services:
  svc0:
    build: .
    image: %s
`, imageSvc0)

	dockerfile := fmt.Sprintf(`FROM %s`, testutil.AlpineImage)

	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	comp.WriteFile("Dockerfile", dockerfile)
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	defer base.Cmd("rmi", imageSvc0).Run()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	// `compose create --no-build` should fail if service image needs build
	base.ComposeCmd("-f", comp.YAMLFullPath(), "create", "--no-build").AssertFail()
	// `compose create --build` should succeed: image is built and container is created
	base.ComposeCmd("-f", comp.YAMLFullPath(), "create", "--build").AssertOK()
	base.ComposeCmd("-f", comp.YAMLFullPath(), "images", "svc0").AssertOutContains(imageSvc0)
	base.ComposeCmd("-f", comp.YAMLFullPath(), "ps", "svc0", "-a").AssertOutContainsAny("Created", "created")
}
