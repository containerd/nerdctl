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
	"errors"
	"fmt"
	"net"
	"runtime"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestComposeExec(t *testing.T) {
	base := testutil.NewBase(t)
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
    command: "sleep infinity"
  svc1:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d", "svc0").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	// test basic functionality and `--workdir` flag
	base.ComposeCmd("-f", comp.YAMLFullPath(), "exec", "-i=false", "--no-TTY", "svc0", "echo", "success").AssertOutExactly("success\n")
	base.ComposeCmd("-f", comp.YAMLFullPath(), "exec", "-i=false", "--no-TTY", "--workdir", "/tmp", "svc0", "pwd").AssertOutExactly("/tmp\n")
	// cannot `exec` on non-running service
	base.ComposeCmd("-f", comp.YAMLFullPath(), "exec", "svc1", "echo", "success").AssertFail()
}

func TestComposeExecWithEnv(t *testing.T) {
	base := testutil.NewBase(t)
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	// FYI: https://github.com/containerd/nerdctl/blob/e4b2b6da56555dc29ed66d0fd8e7094ff2bc002d/cmd/nerdctl/run_test.go#L177
	base.Env = append(base.Env, "CORGE=corge-value-in-host", "GARPLY=garply-value-in-host")
	base.ComposeCmd("-f", comp.YAMLFullPath(), "exec", "-i=false", "--no-TTY",
		"--env", "FOO=foo1,foo2",
		"--env", "BAR=bar1 bar2",
		"--env", "BAZ=",
		"--env", "QUX", // not exported in OS
		"--env", "QUUX=quux1",
		"--env", "QUUX=quux2",
		"--env", "CORGE", // OS exported
		"--env", "GRAULT=grault_key=grault_value", // value contains `=` char
		"--env", "GARPLY=", // OS exported
		"--env", "WALDO=", // not exported in OS

		"svc0", "env").AssertOutWithFunc(func(stdout string) error {
		if !strings.Contains(stdout, "\nFOO=foo1,foo2\n") {
			return errors.New("got bad FOO")
		}
		if !strings.Contains(stdout, "\nBAR=bar1 bar2\n") {
			return errors.New("got bad BAR")
		}
		if !strings.Contains(stdout, "\nBAZ=\n") && runtime.GOOS != "windows" {
			return errors.New("got bad BAZ")
		}
		if strings.Contains(stdout, "QUX") {
			return errors.New("got bad QUX (should not be set)")
		}
		if !strings.Contains(stdout, "\nQUUX=quux2\n") {
			return errors.New("got bad QUUX")
		}
		if !strings.Contains(stdout, "\nCORGE=corge-value-in-host\n") {
			return errors.New("got bad CORGE")
		}
		if !strings.Contains(stdout, "\nGRAULT=grault_key=grault_value\n") {
			return errors.New("got bad GRAULT")
		}
		if !strings.Contains(stdout, "\nGARPLY=\n") && runtime.GOOS != "windows" {
			return errors.New("got bad GARPLY")
		}
		if !strings.Contains(stdout, "\nWALDO=\n") && runtime.GOOS != "windows" {
			return errors.New("got bad WALDO")
		}

		return nil
	})
}

func TestComposeExecWithUser(t *testing.T) {
	base := testutil.NewBase(t)
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
    command: "sleep infinity"
`, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()

	testCases := map[string]string{
		"":             "uid=0(root) gid=0(root)",
		"1000":         "uid=1000 gid=0(root)",
		"1000:users":   "uid=1000 gid=100(users)",
		"guest":        "uid=405(guest) gid=100(users)",
		"nobody":       "uid=65534(nobody) gid=65534(nobody)",
		"nobody:users": "uid=65534(nobody) gid=100(users)",
	}

	for userStr, expected := range testCases {
		args := []string{"-f", comp.YAMLFullPath(), "exec", "-i=false", "--no-TTY"}
		if userStr != "" {
			args = append(args, "--user", userStr)
		}
		args = append(args, "svc0", "id")
		base.ComposeCmd(args...).AssertOutContains(expected)
	}
}

func TestComposeExecTTY(t *testing.T) {
	// `-i` in `compose run & exec` is only supported in compose v2.
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
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(), "exec", "--no-TTY", "svc0", "stty").AssertFail()                         // `-i`
	base.ComposeCmdWithHelper(unbuffer, "-f", comp.YAMLFullPath(), "exec", "-i=false", "--no-TTY", "svc0", "stty").AssertFail()
}

func TestComposeExecWithIndex(t *testing.T) {
	base := testutil.NewBase(t)
	var dockerComposeYAML = fmt.Sprintf(`
version: '3.1'

services:
  svc0:
    image: %s
    command: "sleep infinity"
    deploy:
      replicas: 3
`, testutil.CommonImage)

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	t.Cleanup(func() {
		comp.CleanUp()
	})
	projectName := comp.ProjectName()
	t.Logf("projectName=%q", projectName)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d", "svc0").AssertOK()
	t.Cleanup(func() {
		base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").AssertOK()
	})

	// try 5 times to ensure that results are stable
	for i := 0; i < 5; i++ {
		for _, j := range []string{"1", "2", "3"} {
			name := fmt.Sprintf("%s-svc0-%s", projectName, j)
			host := fmt.Sprintf("%s.%s_default", name, projectName)
			var (
				expectIP string
				realIP   string
			)
			//  docker and nerdctl have different DNS resolution behaviors.
			// it uses the ID in the /etc/hosts file, so we need to fetch the ID first.
			if testutil.GetTarget() == testutil.Docker {
				base.Cmd("ps", "--filter", fmt.Sprintf("name=%s", name), "--format", "{{.ID}}").AssertOutWithFunc(func(stdout string) error {
					host = strings.TrimSpace(stdout)
					return nil
				})
			}
			cmds := []string{"-f", comp.YAMLFullPath(), "exec", "-i=false", "--no-TTY", "--index", j, "svc0"}
			base.ComposeCmd(append(cmds, "cat", "/etc/hosts")...).
				AssertOutWithFunc(func(stdout string) error {
					lines := strings.Split(stdout, "\n")
					for _, line := range lines {
						if !strings.Contains(line, host) {
							continue
						}
						fields := strings.Fields(line)
						if len(fields) == 0 {
							continue
						}
						expectIP = fields[0]
						return nil
					}
					return errors.New("fail to get the expected ip address")
				})
			base.ComposeCmd(append(cmds, "ip", "addr", "show", "dev", "eth0")...).
				AssertOutWithFunc(func(stdout string) error {
					ip := findIP(stdout)
					if ip == nil {
						return errors.New("fail to get the real ip address")
					}
					realIP = ip.String()
					return nil
				})
			assert.Equal(t, realIP, expectIP)
		}
	}
}

func findIP(output string) net.IP {
	var ip string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if !strings.Contains(line, "inet ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) <= 1 {
			continue
		}
		ip = strings.Split(fields[1], "/")[0]
		break
	}
	return net.ParseIP(ip)
}
