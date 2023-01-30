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
	"runtime"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/icmd"
)

// https://github.com/containerd/nerdctl/issues/1942
func TestComposeUpDetailedError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("FIXME: test does not work on Windows yet (runtime \"io.containerd.runc.v2\" binary not installed \"containerd-shim-runc-v2.exe\": file does not exist)")
	}
	base := testutil.NewBase(t)
	dockerComposeYAML := fmt.Sprintf(`
services:
  foo:
    image: %s
    runtime: invalid
`, testutil.CommonImage)
	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	c := base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d")
	expected := icmd.Expected{
		ExitCode: 1,
		Err:      `exec: \"invalid\": executable file not found in $PATH`,
	}
	if base.Target == testutil.Docker {
		expected.Err = `Unknown runtime specified invalid`
	}
	c.Assert(expected)
}
