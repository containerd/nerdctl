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
	"fmt"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestStartDetachKeys(t *testing.T) {
	testutil.RequireExecutable(t, "unbuffer")

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

func TestStartInteractive(t *testing.T) {
	testutil.RequireExecutable(t, "unbuffer")

	t.Parallel()

	containerName := testutil.Identifier(t)

	needle := "expect to be echo-ed from stdin"
	expectContainerName := "expect container name"

	tests := []struct {
		baseOp           string
		baseInteractive  bool
		baseTTY          bool
		baseAttach       string
		startInteractive bool
		startAttach      bool
		stdOutExpect     string
	}{
		{
			"create",
			false,
			false,
			"",
			false,
			false,
			expectContainerName,
		},
		{
			"create",
			false,
			false,
			"",
			true,
			false,
			"",
		},
		{
			"create",
			true,
			false,
			"",
			false,
			false,
			expectContainerName,
		},
		{
			"create",
			true,
			false,
			"",
			true,
			false,
			needle,
		},
		{
			"create",
			false,
			true,
			"",
			false,
			false,
			expectContainerName,
		},
		/*
			// Not clear what is supposed to happen in that case
			{
				"create",
				false,
				true,
				"",
				true,
				false,
				"",
			},

		*/
		{
			"create",
			true,
			true,
			"",
			false,
			false,
			expectContainerName,
		},
		/*
			// Not clear what is supposed to happen in that case
				{
					"create",
					true,
					true,
					"",
					true,
					false,
					needle + "\n",
				},

		*/
	}

	for _, tt := range tests {
		name := containerName + fmt.Sprintf("-op_%s-int_%t-tty_%t-att%s-sint_%t-satt_%t", tt.baseOp, tt.baseInteractive, tt.baseTTY, tt.baseAttach, tt.startInteractive, tt.startAttach)
		args := []string{tt.baseOp, "--name", name}
		stdOutExpect := tt.stdOutExpect
		helper := []string{}
		if stdOutExpect == expectContainerName {
			stdOutExpect = name
		}
		if tt.baseInteractive {
			args = append(args, "--interactive")
		}

		in := []byte("echo " + needle)

		/*
			if tt.baseTTY {
				args = append(args, "--tty")
				if tt.startInteractive {
					helper = []string{"unbuffer", "-p"}
					in = append(in, 1, 2)
					if stdOutExpect != "" {
						// stdOutExpect = stdOutExpect + string([]byte{1, 2})
					}
				}
			}
		*/

		if stdOutExpect != "" {
			stdOutExpect += "\n"
		}
		if tt.baseAttach != "" {
			args = append(args, "--attach", tt.baseAttach)
		}

		startArgs := []string{"container", "start", "--detach-keys=ctrl-a,ctrl-b"}
		if tt.startInteractive {
			startArgs = append(startArgs, "--interactive")
		}
		if tt.startAttach {
			startArgs = append(startArgs, "--attach")
		}
		startArgs = append(startArgs, name)

		t.Run(name, func(tes *testing.T) {
			base := testutil.NewBase(t)
			tearDown := func() {
				base.Cmd("container", "rm", "-f", name).Run()
			}

			tearDown()
			tes.Cleanup(tearDown)

			args = append(args, testutil.CommonImage)
			base.Cmd(args...).AssertOK()

			base.CmdWithHelper(helper, startArgs...).
				CmdOption(testutil.WithStdin(bytes.NewReader(in))).
				AssertOutExactly(stdOutExpect)
		})
	}
}
