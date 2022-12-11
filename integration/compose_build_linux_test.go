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
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestComposeBuild(t *testing.T) {
	const imageSvc0 = "composebuild_svc0"
	const imageSvc1 = "composebuild_svc1"

	dockerComposeYAML := fmt.Sprintf(`
services:
  svc0:
    build: .
    image: %s
    ports:
    - 8080:80
  svc1:
    build: .
    image: %s
    ports:
    - 8081:80
`, imageSvc0, imageSvc1)

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
	defer base.Cmd("rmi", imageSvc1).Run()

	// 1. build only 1 service
	base.ComposeCmd("-f", comp.YAMLFullPath(), "build", "svc0").AssertOK()
	base.Cmd("images").AssertOutContains(imageSvc0)
	base.Cmd("images").AssertOutNotContains(imageSvc1)
	// 2. build multiple services
	base.ComposeCmd("-f", comp.YAMLFullPath(), "build", "svc0", "svc1").AssertOK()
	base.Cmd("images").AssertOutContains(imageSvc0)
	base.Cmd("images").AssertOutContains(imageSvc1)
	// 3. build all if no args are given
	base.ComposeCmd("-f", comp.YAMLFullPath(), "build").AssertOK()
	// 4. fail if some services args not exist in compose.yml
	base.ComposeCmd("-f", comp.YAMLFullPath(), "build", "svc0", "svc100").AssertFail()
}
