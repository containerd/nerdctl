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
