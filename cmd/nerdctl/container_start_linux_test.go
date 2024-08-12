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
	"bytes"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

func TestStartDetachKeys(t *testing.T) {
	t.Parallel()

	skipAttachForDocker(t)

	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	defer base.Cmd("container", "rm", "-f", containerName).AssertOK()
	opts := []func(*testutil.Cmd){
		// If NewDelayOnceReader is not used,
		// the container state will be Created instead of Exited.
		// Maybe `unbuffer` exits too early in that case?
		testutil.WithStdin(testutil.NewDelayOnceReader(strings.NewReader("exit\n"))),
	}
	// unbuffer(1) emulates tty, which is required by `nerdctl run -t`.
	// unbuffer(1) can be installed with `apt-get install expect`.
	//
	// "-p" is needed because we need unbuffer to read from stdin, and from [1]:
	// "Normally, unbuffer does not read from stdin. This simplifies use of unbuffer in some situations.
	//  To use unbuffer in a pipeline, use the -p flag."
	//
	// [1] https://linux.die.net/man/1/unbuffer
	base.CmdWithHelper([]string{"unbuffer", "-p"}, "run", "-it", "--name", containerName, testutil.CommonImage).
		CmdOption(opts...).AssertOK()
	container := base.InspectContainer(containerName)
	assert.Equal(base.T, container.State.Running, false)

	opts = []func(*testutil.Cmd){
		testutil.WithStdin(testutil.NewDelayOnceReader(bytes.NewReader([]byte{1, 2}))), // https://www.physics.udel.edu/~watson/scen103/ascii.html
	}
	base.CmdWithHelper([]string{"unbuffer", "-p"}, "start", "-a", "--detach-keys=ctrl-a,ctrl-b", containerName).
		CmdOption(opts...).AssertOutContains("read detach keys")
	container = base.InspectContainer(containerName)
	assert.Equal(base.T, container.State.Running, true)
}
