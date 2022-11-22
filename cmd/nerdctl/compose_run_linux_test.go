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
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/containerd/nerdctl/pkg/testutil/nettestutil"
	"github.com/containerd/nerdctl/pkg/testutil/testregistry"
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
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	const sttyPartialOutput = "speed 38400 baud"
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
		"run", "--name", containerName, "alpine").AssertOutContains(sttyPartialOutput)
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
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	const sttyPartialOutput = "speed 38400 baud"
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
		"run", "--name", containerName, "--rm", "alpine").AssertOutContains(sttyPartialOutput)

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
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	go func() {
		// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
		// unbuffer(1) can be installed with `apt-get install expect`.
		unbuffer := []string{"unbuffer"}
		base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
			"run", "--service-ports", "--name", containerName, "web").Run()
	}()

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
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	go func() {
		// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
		// unbuffer(1) can be installed with `apt-get install expect`.
		unbuffer := []string{"unbuffer"}
		base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
			"run", "--publish", "8080:80", "--name", containerName, "web").Run()
	}()

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
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	const partialOutput = "bar"
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
		"run", "-e", "FOO=bar", "--name", containerName, "alpine").AssertOutContains(partialOutput)
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
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	const partialOutput = "5000"
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
		"run", "--user", "5000", "--name", containerName, "alpine").AssertOutContains(partialOutput)
}

func TestComposeRunWithLabel(t *testing.T) {
	base := testutil.NewBase(t)
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
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
		"run", "--label", "foo=rab", "--label", "x=y", "--name", containerName, "alpine").AssertOK()

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
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	const partialOutput = "hello world"
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
		"run", "--name", containerName, "alpine", partialOutput).AssertOutContains(partialOutput)
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
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	const partialOutput = "hello world"
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
		"run", "--entrypoint", "echo", "--name", containerName, "alpine", partialOutput).AssertOutContains(partialOutput)
}

func TestComposeRunWithVolume(t *testing.T) {
	base := testutil.NewBase(t)
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
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	// The directory is automatically removed by Cleanup
	tmpDir := t.TempDir()
	destinationDir := "/data"
	volumeFlagStr := fmt.Sprintf("%s:%s", tmpDir, destinationDir)

	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(),
		"run", "--volume", volumeFlagStr, "--name", containerName, "alpine").AssertOK()

	container := base.InspectContainer(containerName)
	errMsg := fmt.Sprintf("test failed, cannot find volume: %v", container.Mounts)
	assert.Assert(t, container.Mounts != nil, errMsg)
	assert.Assert(t, len(container.Mounts) == 1, errMsg)
	assert.Assert(t, container.Mounts[0].Source == tmpDir, errMsg)
	assert.Assert(t, container.Mounts[0].Destination == destinationDir, errMsg)
}

func TestComposePushAndPullWithCosignVerify(t *testing.T) {
	if _, err := exec.LookPath("cosign"); err != nil {
		t.Skip()
	}
	testutil.DockerIncompatible(t)
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()

	// set up cosign and local registry
	t.Setenv("COSIGN_PASSWORD", "1")
	keyPair := newCosignKeyPair(t, "cosign-key-pair")
	defer keyPair.cleanup()

	reg := testregistry.NewPlainHTTP(base, 5000)
	defer reg.Cleanup()
	localhostIP := "127.0.0.1"
	t.Logf("localhost IP=%q", localhostIP)
	testImageRefPrefix := fmt.Sprintf("%s:%d/",
		localhostIP, reg.ListenPort)
	t.Logf("testImageRefPrefix=%q", testImageRefPrefix)

	var (
		imageSvc0 = testImageRefPrefix + "composebuild_svc0"
		imageSvc1 = testImageRefPrefix + "composebuild_svc1"
		imageSvc2 = testImageRefPrefix + "composebuild_svc2"
	)

	dockerComposeYAML := fmt.Sprintf(`
services:
  svc0:
    build: .
    image: %s
    x-nerdctl-verify: cosign
    x-nerdctl-cosign-public-key: %s
    x-nerdctl-sign: cosign
    x-nerdctl-cosign-private-key: %s
    entrypoint:
      - stty
  svc1:
    build: .
    image: %s
    x-nerdctl-verify: cosign
    x-nerdctl-cosign-public-key: dummy_pub_key
    x-nerdctl-sign: cosign
    x-nerdctl-cosign-private-key: %s
    entrypoint:
      - stty
  svc2:
    build: .
    image: %s
    x-nerdctl-verify: none
    x-nerdctl-sign: none
    entrypoint:
      - stty
`, imageSvc0, keyPair.publicKey, keyPair.privateKey,
		imageSvc1, keyPair.privateKey, imageSvc2)

	dockerfile := fmt.Sprintf(`FROM %s`, testutil.AlpineImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	comp.WriteFile("Dockerfile", dockerfile)

	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	// 1. build both services/images
	base.ComposeCmd("-f", comp.YAMLFullPath(), "build").AssertOK()
	// 2. compose push with cosign for svc0/svc1, (and none for svc2)
	base.ComposeCmd("-f", comp.YAMLFullPath(), "push").AssertOK()
	// 3. compose pull with cosign
	base.ComposeCmd("-f", comp.YAMLFullPath(), "pull", "svc0").AssertOK()   // key match
	base.ComposeCmd("-f", comp.YAMLFullPath(), "pull", "svc1").AssertFail() // key mismatch
	base.ComposeCmd("-f", comp.YAMLFullPath(), "pull", "svc2").AssertOK()   // verify passed
	// 4. compose run
	const sttyPartialOutput = "speed 38400 baud"
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	unbuffer := []string{"unbuffer"}
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(), "run", "svc0").AssertOutContains(sttyPartialOutput) // key match
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(), "run", "svc1").AssertFail()                         // key mismatch
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(), "run", "svc2").AssertOutContains(sttyPartialOutput) // verify passed
	// 5. compose up
	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "svc0").AssertOK()   // key match
	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "svc1").AssertFail() // key mismatch
	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "svc2").AssertOK()   // verify passed
}
