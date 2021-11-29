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
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
)

func TestExec(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	const testContainer = "nerdctl-test-exec"
	defer base.Cmd("rm", "-f", testContainer).Run()

	base.Cmd("run", "-d", "--name", testContainer, testutil.CommonImage, "sleep", "1h").AssertOK()

	base.Cmd("exec", testContainer, "echo", "success").AssertOutExactly("success\n")
}

func TestExecWithDoubleDash(t *testing.T) {
	t.Parallel()
	testutil.DockerIncompatible(t)
	base := testutil.NewBase(t)
	const testContainer = "nerdctl-test-exec-double-dash"
	defer base.Cmd("rm", "-f", testContainer).Run()

	base.Cmd("run", "-d", "--name", testContainer, testutil.CommonImage, "sleep", "1h").AssertOK()

	base.Cmd("exec", testContainer, "--", "echo", "success").AssertOutExactly("success\n")
}
