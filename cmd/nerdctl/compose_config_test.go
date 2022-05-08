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
	"os"
	"path/filepath"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestComposeConfig(t *testing.T) {
	base := testutil.NewBase(t)

	var dockerComposeYAML = `
services:
  hello:
    image: alpine:3.13
`

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	base.ComposeCmd("-f", comp.YAMLFullPath(), "config").AssertOutContains("hello:")
}

func TestComposeConfigWithPrintService(t *testing.T) {
	base := testutil.NewBase(t)

	var dockerComposeYAML = `
services:
  hello1:
    image: alpine:3.13
`

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	base.ComposeCmd("-f", comp.YAMLFullPath(), "config", "--services").AssertOutExactly("hello1\n")
}

func TestComposeConfigWithPrintServiceHash(t *testing.T) {
	base := testutil.NewBase(t)

	var dockerComposeYAML = `
services:
  hello1:
    image: alpine:%s
`

	comp := testutil.NewComposeDir(t, fmt.Sprintf(dockerComposeYAML, "3.13"))
	defer comp.CleanUp()

	base.ComposeCmd("-f", comp.YAMLFullPath(), "config", "--hash=*").AssertOutContains("hello1")
	hash := base.ComposeCmd("-f", comp.YAMLFullPath(), "config", "--hash=hello1").Out()

	newComp := testutil.NewComposeDir(t, fmt.Sprintf(dockerComposeYAML, "3.14"))
	defer newComp.CleanUp()

	newHash := base.ComposeCmd("-f", newComp.YAMLFullPath(), "config", "--hash=hello1").Out()
	assert.Assert(t, hash != newHash)
}

func TestComposeConfigWithMultipleFile(t *testing.T) {
	base := testutil.NewBase(t)

	var dockerComposeYAML = `
services:
  hello1:
    image: alpine:3.13
`

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	comp.WriteFile("docker-compose.test.yml", `
services:
  hello2:
    image: alpine:3.14
`)
	comp.WriteFile("docker-compose.override.yml", `
services:
  hello1:
    image: alpine:3.14
`)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "-f", filepath.Join(comp.Dir(), "docker-compose.test.yml"), "config").AssertOutContains("alpine:3.14")
	base.ComposeCmd("--project-directory", comp.Dir(), "config", "--services").AssertOutExactly("hello1\n")
	base.ComposeCmd("--project-directory", comp.Dir(), "config").AssertOutContains("alpine:3.14")
}

func TestComposeConfigWithComposeFileEnv(t *testing.T) {
	base := testutil.NewBase(t)

	var dockerComposeYAML = `
services:
  hello1:
    image: alpine:3.13
`

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	comp.WriteFile("docker-compose.test.yml", `
services:
  hello2:
    image: alpine:3.14
`)

	base.Env = append(os.Environ(), "COMPOSE_FILE="+comp.YAMLFullPath()+","+filepath.Join(comp.Dir(), "docker-compose.test.yml"), "COMPOSE_PATH_SEPARATOR=,")

	base.ComposeCmd("config").AssertOutContains("alpine:3.14")
	base.ComposeCmd("--project-directory", comp.Dir(), "config", "--services").AssertOutExactly("hello1\nhello2\n")
	base.ComposeCmd("--project-directory", comp.Dir(), "config").AssertOutContains("alpine:3.14")
}
