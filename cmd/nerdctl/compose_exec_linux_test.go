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

func TestComposeExec(t *testing.T) {
	// `-i` in `compose run & exec` is only supported in compose v2.
	// Currently CI is using compose v1.
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
  svc1:
    image: %s
`, testutil.CommonImage, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "run", "-d", "-i=false", "svc0", "sleep", "1h").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	base.ComposeCmd("-f", comp.YAMLFullPath(), "exec", "-i=false", "-t=false", "svc0", "echo", "success").AssertOutExactly("success\n")
	base.ComposeCmd("-f", comp.YAMLFullPath(), "exec", "-i=false", "-t=false", "--workdir", "/tmp", "svc0", "pwd").AssertOutExactly("/tmp\n")
	base.ComposeCmd("-f", comp.YAMLFullPath(), "exec", "-i=false", "-t=false", "svc1", "echo", "success").AssertFail()
}

func TestComposeExecWithUser(t *testing.T) {
	// `-i` in `compose run & exec` is only supported in compose v2.
	// Currently CI is using compose v1.
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
  svc1:
    image: %s
`, testutil.CommonImage, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	testContainer := testutil.Identifier(t)
	base.ComposeCmd("-f", comp.YAMLFullPath(), "run", "-d", "-i=false", "--name", testContainer, "svc0", "sleep", "1h").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()
	base.EnsureContainerStarted(testContainer)

	testCases := map[string]string{
		"":             "uid=0(root) gid=0(root)",
		"1000":         "uid=1000 gid=0(root)",
		"1000:users":   "uid=1000 gid=100(users)",
		"guest":        "uid=405(guest) gid=100(users)",
		"nobody":       "uid=65534(nobody) gid=65534(nobody)",
		"nobody:users": "uid=65534(nobody) gid=100(users)",
	}

	for userStr, expected := range testCases {
		args := []string{"-f", comp.YAMLFullPath(), "exec", "-i=false", "-t=false"}
		if userStr != "" {
			args = append(args, "--user", userStr)
		}
		args = append(args, "svc0", "id")
		base.ComposeCmd(args...).AssertOutContains(expected)
	}
}

func TestComposeExecTTY(t *testing.T) {
	// `-i` in `compose run & exec` is only supported in compose v2.
	// Currently CI is using compose v1.
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	if testutil.GetTarget() == testutil.Nerdctl {
		testutil.RequireDaemonVersion(base, ">= 1.6.0-0")
	}

	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
  svc1:
    image: %s
`, testutil.CommonImage, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	testContainer := testutil.Identifier(t)
	base.ComposeCmd("-f", comp.YAMLFullPath(), "run", "-d", "-i=false", "--name", testContainer, "svc0", "sleep", "1h").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()
	base.EnsureContainerStarted(testContainer)

	const sttyPartialOutput = "speed 38400 baud"
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(), "exec", "svc0", "stty").AssertOutContains(sttyPartialOutput)             // `-it`
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(), "exec", "-i=false", "svc0", "stty").AssertOutContains(sttyPartialOutput) // `-t`
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(), "exec", "-t=false", "svc0", "stty").AssertFail()                         // `-i`
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(), "exec", "-i=false", "-t=false", "svc0", "stty").AssertFail()
}
