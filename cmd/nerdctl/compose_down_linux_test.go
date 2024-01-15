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

	"github.com/containerd/nerdctl/v2/pkg/composer/serviceparser"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestComposeDownRemoveUsedNetwork(t *testing.T) {
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

	base.ComposeCmd("-p", projectName, "-f", compFull.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-p", projectName, "-f", compFull.YAMLFullPath(), "down", "--remove-orphans").AssertOK()

	base.ComposeCmd("-p", projectName, "-f", compOrphan.YAMLFullPath(), "down", "-v").AssertCombinedOutContains("in use")

}

func TestComposeDownRemoveOrphans(t *testing.T) {
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

	projectName := compFull.ProjectName()
	t.Logf("projectName=%q", projectName)

	orphanContainer := serviceparser.DefaultContainerName(projectName, "orphan", "1")

	base.ComposeCmd("-p", projectName, "-f", compFull.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-p", projectName, "-f", compFull.YAMLFullPath(), "down", "-v").Run()

	base.ComposeCmd("-p", projectName, "-f", compOrphan.YAMLFullPath(), "down", "--remove-orphans").AssertOK()
	base.ComposeCmd("-p", projectName, "-f", compFull.YAMLFullPath(), "ps", "-a").AssertOutNotContains(orphanContainer)
}
