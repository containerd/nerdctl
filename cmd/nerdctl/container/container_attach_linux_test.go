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

package container

import (
	"bytes"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

// skipAttachForDocker should be called by attach-related tests that assert 'read detach keys' in stdout.
func skipAttachForDocker(t *testing.T) {
	t.Helper()
	if testutil.GetTarget() == testutil.Docker {
		t.Skip("When detaching from a container, for a session started with 'docker attach'" +
			", it prints 'read escape sequence', but for one started with 'docker (run|start)', it prints nothing." +
			" However, the flag is called '--detach-keys' in all cases" +
			", so nerdctl prints 'read detach keys' for all cases" +
			", and that's why this test is skipped for Docker.")
	}
}

// prepareContainerToAttach spins up a container (entrypoint = shell) with `-it` and detaches from it
// so that it can be re-attached to later.
func prepareContainerToAttach(base *testutil.Base, containerName string) {
	opts := []func(*testutil.Cmd){
		testutil.WithStdin(testutil.NewDelayOnceReader(bytes.NewReader(
			[]byte{16, 17}, // ctrl+p,ctrl+q, see https://www.physics.udel.edu/~watson/scen103/ascii.html
		))),
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
		CmdOption(opts...).AssertOutContains("read detach keys")
	container := base.InspectContainer(containerName)
	assert.Equal(base.T, container.State.Running, true)
}

func TestAttach(t *testing.T) {
	t.Parallel()

	skipAttachForDocker(t)

	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	defer base.Cmd("container", "rm", "-f", containerName).AssertOK()
	prepareContainerToAttach(base, containerName)

	opts := []func(*testutil.Cmd){
		testutil.WithStdin(testutil.NewDelayOnceReader(strings.NewReader("expr 1 + 1\nexit\n"))),
	}
	// `unbuffer -p` returns 0 even if the underlying nerdctl process returns a non-zero exit code,
	// so the exit code cannot be easily tested here.
	base.CmdWithHelper([]string{"unbuffer", "-p"}, "attach", containerName).CmdOption(opts...).AssertOutContains("2")
	container := base.InspectContainer(containerName)
	assert.Equal(base.T, container.State.Running, false)
}

func TestAttachDetachKeys(t *testing.T) {
	t.Parallel()

	skipAttachForDocker(t)

	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)

	defer base.Cmd("container", "rm", "-f", containerName).AssertOK()
	prepareContainerToAttach(base, containerName)

	opts := []func(*testutil.Cmd){
		testutil.WithStdin(testutil.NewDelayOnceReader(bytes.NewReader(
			[]byte{1, 2}, // https://www.physics.udel.edu/~watson/scen103/ascii.html
		))),
	}
	base.CmdWithHelper([]string{"unbuffer", "-p"}, "attach", "--detach-keys=ctrl-a,ctrl-b", containerName).
		CmdOption(opts...).AssertOutContains("read detach keys")
	container := base.InspectContainer(containerName)
	assert.Equal(base.T, container.State.Running, true)
}
