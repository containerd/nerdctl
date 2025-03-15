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
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestCreate(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("create", "--name", data.Identifier("container"), testutil.CommonImage, "echo", "foo")
		data.Set("cID", data.Identifier("container"))
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("container"))
	}

	testCase.Require = nerdtest.IsFlaky("https://github.com/containerd/nerdctl/issues/3717")

	testCase.SubTests = []*test.Case{
		{
			Description: "ps -a",
			NoParallel:  true,
			Command:     test.Command("ps", "-a"),
			// FIXME: this might get a false positive if other tests have created a container
			Expected: test.Expects(0, nil, expect.Contains("Created")),
		},
		{
			Description: "start",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("start", data.Get("cID"))
			},
			Expected: test.Expects(0, nil, nil),
		},
		{
			Description: "logs",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", data.Get("cID"))
			},
			Expected: test.Expects(0, nil, expect.Contains("foo")),
		},
	}

	testCase.Run(t)
}

func TestCreateHyperVContainer(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.HyperV

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("create", "--isolation", "hyperv", "--name", data.Identifier("container"), testutil.CommonImage, "echo", "foo")
		data.Set("cID", data.Identifier("container"))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("container"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "ps -a",
			NoParallel:  true,
			Command:     test.Command("ps", "-a"),
			// FIXME: this might get a false positive if other tests have created a container
			Expected: test.Expects(0, nil, expect.Contains("Created")),
		},
		{
			Description: "start",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("start", data.Get("cID"))
				ran := false
				for i := 0; i < 10 && !ran; i++ {
					helpers.Command("container", "inspect", data.Get("cID")).
						Run(&test.Expected{
							ExitCode: expect.ExitCodeNoCheck,
							Output: func(stdout string, info string, t *testing.T) {
								var dc []dockercompat.Container
								err := json.Unmarshal([]byte(stdout), &dc)
								if err != nil || len(dc) == 0 {
									return
								}
								assert.Equal(t, len(dc), 1, "Unexpectedly got multiple results\n"+info)
								ran = dc[0].State.Status == "exited"
							},
						})
					time.Sleep(time.Second)
				}
				assert.Assert(t, ran, "container did not ran after 10 seconds")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", data.Get("cID"))
			},
			Expected: test.Expects(0, nil, expect.Contains("foo")),
		},
	}

	testCase.Run(t)
}

func TestCreateWithAttach(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("create attach test is not yet implemented on Windows")
	}
	t.Parallel()
	base := testutil.NewBase(t)

	// Test --attach stdout only
	stdoutContainer := testutil.Identifier(t) + "-stdout"
	t.Cleanup(func() {
		base.Cmd("rm", "-f", stdoutContainer).Run()
	})
	base.Cmd("create", "--name", stdoutContainer, "--attach", "stdout",
		testutil.CommonImage, "sh", "-c", "echo stdout-msg && echo stderr-msg >&2").AssertOK()
	base.Cmd("start", stdoutContainer).AssertOutStreamsWithFunc(func(stdout, stderr string) error {
		// Verify stdout message is present
		expected := "stdout-msg"
		if !strings.Contains(stdout, expected) {
			return fmt.Errorf("expected %q, got %q", expected, stdout)
		}
		// Verify stderr message is not present
		if strings.Contains(stderr, "stderr-msg") {
			return fmt.Errorf("stderr message was captured but should not be: %q", stderr)
		}
		return nil
	})

	// Test --attach stderr only
	stderrContainer := testutil.Identifier(t) + "-stderr"
	t.Cleanup(func() {
		base.Cmd("rm", "-f", stderrContainer).Run()
	})
	base.Cmd("create", "--name", stderrContainer, "--attach", "stderr",
		testutil.CommonImage, "sh", "-c", "echo stdout-msg && echo stderr-msg >&2").AssertOK()
	base.Cmd("start", stderrContainer).AssertOutStreamsWithFunc(func(stdout, stderr string) error {
		// Verify stderr message is present
		expected := "stderr-msg"
		if !strings.Contains(stderr, expected) {
			return fmt.Errorf("expected %q, got %q", expected, stderr)
		}
		// Verify stdout message is not present
		if strings.Contains(stdout, "stdout-msg") {
			return fmt.Errorf("stdout message was captured but should not be: %q", stderr)
		}
		return nil
	})

	// Test both stdout and stderr
	bothContainer := testutil.Identifier(t) + "-both"
	t.Cleanup(func() {
		base.Cmd("rm", "-f", bothContainer).Run()
	})
	base.Cmd("create", "--name", bothContainer, "--attach", "stdout", "--attach", "stderr",
		testutil.CommonImage, "sh", "-c", "echo stdout-msg && echo stderr-msg >&2").AssertOK()
	base.Cmd("start", bothContainer).AssertOutStreamsWithFunc(func(stdout, stderr string) error {
		// Verify stdout message is present
		expectedStdout := "stdout-msg"
		if !strings.Contains(stdout, expectedStdout) {
			return fmt.Errorf("Expected stdout message %q, got %q", expectedStdout, stdout)
		}
		// Verify stderr message is present
		expectedStderr := "stderr-msg"
		if !strings.Contains(stderr, expectedStderr) {
			return fmt.Errorf("Expected stderr message %q, got %q", expectedStderr, stderr)
		}
		return nil
	})

	// Test start --attach
	attachContainer := testutil.Identifier(t) + "-attach"
	defer base.Cmd("rm", "-f", attachContainer).Run()
	base.Cmd("create", "--name", attachContainer,
		testutil.CommonImage, "sh", "-c", "echo stdout-msg && echo stderr-msg >&2").AssertOK()
	base.Cmd("start", "--attach", attachContainer).AssertOutStreamsWithFunc(func(stdout, stderr string) error {
		// Verify stdout message is present
		expectedStdout := "stdout-msg"
		if !strings.Contains(stdout, expectedStdout) {
			return fmt.Errorf("Expected stdout message %q, got %q", expectedStdout, stdout)
		}
		// Verify stderr message is present
		expectedStderr := "stderr-msg"
		if !strings.Contains(stderr, expectedStderr) {
			return fmt.Errorf("Expected stderr message %q, got %q", expectedStderr, stderr)
		}
		return nil
	})
}
