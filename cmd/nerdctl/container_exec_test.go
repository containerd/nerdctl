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
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestExec(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	testContainer := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", testContainer).Run()

	base.Cmd("run", "-d", "--name", testContainer, testutil.CommonImage, "sleep", "1h").AssertOK()
	base.EnsureContainerStarted(testContainer)

	base.Cmd("exec", testContainer, "echo", "success").AssertOutExactly("success\n")
}

func TestExecWithDoubleDash(t *testing.T) {
	t.Parallel()
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	testContainer := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", testContainer).Run()

	base.Cmd("run", "-d", "--name", testContainer, testutil.CommonImage, "sleep", "1h").AssertOK()
	base.EnsureContainerStarted(testContainer)

	base.Cmd("exec", testContainer, "--", "echo", "success").AssertOutExactly("success\n")
}

func TestExecStdin(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	if testutil.GetTarget() == testutil.Nerdctl {
		testutil.RequireDaemonVersion(base, ">= 1.6.0-0")
	}

	testContainer := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", testContainer).Run()
	base.Cmd("run", "-d", "--name", testContainer, testutil.CommonImage, "sleep", "1h").AssertOK()
	base.EnsureContainerStarted(testContainer)

	const testStr = "test-exec-stdin"
	opts := []func(*testutil.Cmd){
		testutil.WithStdin(strings.NewReader(testStr)),
	}
	base.Cmd("exec", "-i", testContainer, "cat").CmdOption(opts...).AssertOutExactly(testStr)
}

// FYI: https://github.com/containerd/nerdctl/blob/e4b2b6da56555dc29ed66d0fd8e7094ff2bc002d/cmd/nerdctl/run_test.go#L177
func TestExecEnv(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	testContainer := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", testContainer).Run()

	base.Cmd("run", "-d", "--name", testContainer, testutil.CommonImage, "sleep", "1h").AssertOK()
	base.EnsureContainerStarted(testContainer)

	base.Env = append(os.Environ(), "CORGE=corge-value-in-host", "GARPLY=garply-value-in-host")
	base.Cmd("exec",
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

		testContainer, "env").AssertOutWithFunc(func(stdout string) error {
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
