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
	"strings"
	"testing"
	"time"

	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/containerd/nerdctl/pkg/testutil/nettestutil"
	"github.com/sirupsen/logrus"
	"gotest.tools/v3/assert"
)

func TestComposeRun(t *testing.T) {
	base := testutil.NewBase(t)
	// specify the name of container in order to remove
	// TODO: when `compose rm` is implemented, replace it.
	containerName := testutil.Identifier(t)

	dockerComposeYAML := fmt.Sprintf(`
version: '3.1'
services:
  alpine:
    image: %s
    entrypoint:
      - stty
`, testutil.AlpineImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	const sttyPartialOutput = "speed 38400 baud"
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
		"run", "--name", containerName, "alpine").AssertOutContains(sttyPartialOutput)
	defer base.Cmd("rm", "-f", containerName).AssertOK()
}

func TestComposeRunWithRM(t *testing.T) {
	base := testutil.NewBase(t)
	// specify the name of container in order to remove
	// TODO: when `compose rm` is implemented, replace it.
	containerName := testutil.Identifier(t)

	dockerComposeYAML := fmt.Sprintf(`
version: '3.1'
services:
  alpine:
    image: %s
    entrypoint:
      - stty
`, testutil.AlpineImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	const sttyPartialOutput = "speed 38400 baud"
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
		"run", "--name", containerName, "--rm", "alpine").AssertOutContains(sttyPartialOutput)
	// FIXME: currently, `compose rm` is not supported. so use down to remove volumes and networks
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()
	defer base.Cmd("rm", "-f", containerName)

	psCmd := base.Cmd("ps", "-a", "--format=\"{{.Names}}\"")
	result := psCmd.Run()
	stdoutContent := result.Stdout() + result.Stderr()
	assert.Assert(psCmd.Base.T, result.ExitCode == 0, stdoutContent)
	if strings.Contains(stdoutContent, containerName) {
		logrus.Errorf("test failed, the container %s is not removed", stdoutContent)
		t.Fail()
		return
	}
}

func TestComposeRunWithServicePorts(t *testing.T) {
	base := testutil.NewBase(t)
	// specify the name of container in order to remove
	// TODO: when `compose rm` is implemented, replace it.
	containerName := testutil.Identifier(t)

	dockerComposeYAML := fmt.Sprintf(`
version: '3.1'
services:
  web:
    image: %s
    ports:
      - 8080:80
`, testutil.NginxAlpineImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	go func() {
		// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
		// unbuffer(1) can be installed with `apt-get install expect`.
		unbuffer := []string{"unbuffer"}
		base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
			"run", "--service-ports", "--name", containerName, "web").Run()
	}()
	// FIXME: currently, `compose rm` is not supported. so use down to remove volumes and networks
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()
	defer base.Cmd("rm", "-f", containerName).AssertOK()

	checkNginx := func() error {
		resp, err := nettestutil.HTTPGet("http://127.0.0.1:8080", 10, false)
		if err != nil {
			return err
		}
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		t.Logf("respBody=%q", respBody)
		if !strings.Contains(string(respBody), testutil.NginxAlpineIndexHTMLSnippet) {
			return fmt.Errorf("respBody does not contain %q", testutil.NginxAlpineIndexHTMLSnippet)
		}
		return nil
	}
	var nginxWorking bool
	for i := 0; i < 30; i++ {
		t.Logf("(retry %d)", i)
		err := checkNginx()
		if err == nil {
			nginxWorking = true
			break
		}
		t.Log(err)
		time.Sleep(3 * time.Second)
	}
	if !nginxWorking {
		t.Fatal("nginx is not working")
	}
	t.Log("nginx seems functional")
}

func TestComposeRunWithPublish(t *testing.T) {
	base := testutil.NewBase(t)
	// specify the name of container in order to remove
	// TODO: when `compose rm` is implemented, replace it.
	containerName := testutil.Identifier(t)

	dockerComposeYAML := fmt.Sprintf(`
version: '3.1'
services:
  web:
    image: %s
`, testutil.NginxAlpineImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	go func() {
		// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
		// unbuffer(1) can be installed with `apt-get install expect`.
		unbuffer := []string{"unbuffer"}
		base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
			"run", "--publish", "8080:80", "--name", containerName, "web").Run()
	}()
	defer base.Cmd("rm", "-f", containerName).AssertOK()

	checkNginx := func() error {
		resp, err := nettestutil.HTTPGet("http://127.0.0.1:8080", 10, false)
		if err != nil {
			return err
		}
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		t.Logf("respBody=%q", respBody)
		if !strings.Contains(string(respBody), testutil.NginxAlpineIndexHTMLSnippet) {
			return fmt.Errorf("respBody does not contain %q", testutil.NginxAlpineIndexHTMLSnippet)
		}
		return nil
	}
	var nginxWorking bool
	for i := 0; i < 30; i++ {
		t.Logf("(retry %d)", i)
		err := checkNginx()
		if err == nil {
			nginxWorking = true
			break
		}
		t.Log(err)
		time.Sleep(3 * time.Second)
	}
	if !nginxWorking {
		t.Fatal("nginx is not working")
	}
	t.Log("nginx seems functional")
}

func TestComposeRunWithEnv(t *testing.T) {
	base := testutil.NewBase(t)
	// specify the name of container in order to remove
	// TODO: when `compose rm` is implemented, replace it.
	containerName := testutil.Identifier(t)

	dockerComposeYAML := fmt.Sprintf(`
version: '3.1'
services:
  alpine:
    image: %s
    entrypoint:
      - sh
      - -c
      - "echo $$FOO"
`, testutil.AlpineImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	const partialOutput = "bar"
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
		"run", "-e", "FOO=bar", "--name", containerName, "alpine").AssertOutContains(partialOutput)
	// FIXME: currently, `compose rm` is not supported. so use down to remove volumes and networks
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()
	defer base.Cmd("rm", "-f", containerName).AssertOK()
}

func TestComposeRunWithUser(t *testing.T) {
	base := testutil.NewBase(t)
	// specify the name of container in order to remove
	// TODO: when `compose rm` is implemented, replace it.
	containerName := testutil.Identifier(t)

	dockerComposeYAML := fmt.Sprintf(`
version: '3.1'
services:
  alpine:
    image: %s
    entrypoint:
      - id
      - -u
`, testutil.AlpineImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	const partialOutput = "5000"
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
		"run", "--user", "5000", "--name", containerName, "alpine").AssertOutContains(partialOutput)
	// FIXME: currently, `compose rm` is not supported. so use down to remove volumes and networks
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()
	defer base.Cmd("rm", "-f", containerName).AssertOK()
}

func TestComposeRunWithLabel(t *testing.T) {
	base := testutil.NewBase(t)
	// specify the name of container in order to remove
	// TODO: when `compose rm` is implemented, replace it.
	containerName := testutil.Identifier(t)

	dockerComposeYAML := fmt.Sprintf(`
version: '3.1'
services:
  alpine:
    image: %s
    entrypoint:
      - echo
      - "dummy log"
    labels:
      - "foo=bar"
`, testutil.AlpineImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
		"run", "--label", "foo=rab", "--label", "x=y", "--name", containerName, "alpine").AssertOK()
	// FIXME: currently, `compose rm` is not supported. so use down to remove volumes and networks
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()
	defer base.Cmd("rm", "-f", containerName).AssertOK()

	container := base.InspectContainer(containerName)
	if container.Config == nil {
		logrus.Errorf("test failed, cannot fetch container config")
		t.Fail()
	}
	assert.Equal(t, container.Config.Labels["foo"], "rab")
	assert.Equal(t, container.Config.Labels["x"], "y")
}

func TestComposeRunWithArgs(t *testing.T) {
	base := testutil.NewBase(t)
	// specify the name of container in order to remove
	// TODO: when `compose rm` is implemented, replace it.
	containerName := testutil.Identifier(t)

	dockerComposeYAML := fmt.Sprintf(`
version: '3.1'
services:
  alpine:
    image: %s
    entrypoint:
      - echo
`, testutil.AlpineImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	const partialOutput = "hello world"
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
		"run", "--name", containerName, "alpine", partialOutput).AssertOutContains(partialOutput)
	// FIXME: currently, `compose rm` is not supported. so use down to remove volumes and networks
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()
	defer base.Cmd("rm", "-f", containerName).AssertOK()
}

func TestComposeRunWithEntrypoint(t *testing.T) {
	base := testutil.NewBase(t)
	// specify the name of container in order to remove
	// TODO: when `compose rm` is implemented, replace it.
	containerName := testutil.Identifier(t)

	dockerComposeYAML := fmt.Sprintf(`
version: '3.1'
services:
  alpine:
    image: %s
    entrypoint:
      - stty # should be changed
`, testutil.AlpineImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	const partialOutput = "hello world"
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
		"run", "--entrypoint", "echo", "--name", containerName, "alpine", partialOutput).AssertOutContains(partialOutput)
	// FIXME: currently, `compose rm` is not supported. so use down to remove volumes and networks
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()
	defer base.Cmd("rm", "-f", containerName).AssertOK()
}

func TestComposeRunWithVolume(t *testing.T) {
	base := testutil.NewBase(t)
	// specify the name of container in order to remove
	// TODO: when `compose rm` is implemented, replace it.
	containerName := testutil.Identifier(t)

	dockerComposeYAML := fmt.Sprintf(`
version: '3.1'
services:
  alpine:
    image: %s
    entrypoint:
    - stty # no meaning, just put any command
`, testutil.AlpineImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	// The directory is automatically removed by Cleanup
	tmpDir := t.TempDir()
	destinationDir := "/data"
	volumeFlagStr := fmt.Sprintf("%s:%s", tmpDir, destinationDir)

	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
		"run", "--volume", volumeFlagStr, "--name", containerName, "alpine").AssertOK()
	// FIXME: currently, `compose rm` is not supported. so use down to remove volumes and networks
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()
	defer base.Cmd("rm", "-f", containerName).AssertOK()

	container := base.InspectContainer(containerName)
	errMsg := fmt.Sprintf("test failed, cannot find volume: %v", container.Mounts)
	assert.Assert(t, container.Mounts != nil, errMsg)
	assert.Assert(t, len(container.Mounts) == 1, errMsg)
	assert.Assert(t, container.Mounts[0].Source == tmpDir, errMsg)
	assert.Assert(t, container.Mounts[0].Destination == destinationDir, errMsg)
}
