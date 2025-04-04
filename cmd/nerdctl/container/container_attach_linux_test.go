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
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

/*
Important notes:
- for both docker and nerdctl, you can run+detach of a container and exit 0, while the container would actually fail starting
- nerdctl (not docker): on run, detach will race anything on stdin before the detach sequence from reaching the container
- nerdctl AND docker: on attach ^
- exit code variants: https://github.com/containerd/nerdctl/issues/3571
*/

func TestAttach(t *testing.T) {
	// In nerdctl the detach return code from the container after attach is 0, but in docker the return code is 1.
	// This behaviour is reported in https://github.com/containerd/nerdctl/issues/3571
	ex := 0
	if nerdtest.IsDocker() {
		ex = 1
	}

	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		cmd := helpers.Command("run", "--rm", "-it", "--name", data.Identifier(), testutil.CommonImage)
		cmd.WithPseudoTTY()
		// ctrl+p and ctrl+q (see https://en.wikipedia.org/wiki/C0_and_C1_control_codes)
		cmd.Feed(bytes.NewReader([]byte{16, 17}))

		cmd.Run(&test.Expected{
			ExitCode: 0,
			Errors:   []error{errors.New("read detach keys")},
			Output: func(stdout string, info string, t *testing.T) {
				assert.Assert(t, strings.Contains(helpers.Capture("inspect", "--format", "json", data.Identifier()), "\"Running\":true"))
			},
		})
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// Run interactively and detach
		cmd := helpers.Command("attach", data.Identifier())

		cmd.WithPseudoTTY()
		cmd.Feed(strings.NewReader("echo mark${NON}mark\n"))
		cmd.WithFeeder(func() io.Reader {
			// Interestingly, and unlike with run, on attach, docker (like nerdctl) ALSO needs a pause so that the
			// container can read stdin before we detach
			time.Sleep(time.Second)
			// ctrl+p and ctrl+q (see https://en.wikipedia.org/wiki/C0_and_C1_control_codes)
			return bytes.NewReader([]byte{16, 17})
		})

		return cmd
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: ex,
			Errors:   []error{errors.New("read detach keys")},
			Output: expect.All(
				expect.Contains("markmark"),
				func(stdout string, info string, t *testing.T) {
					assert.Assert(t, strings.Contains(helpers.Capture("inspect", "--format", "json", data.Identifier()), "\"Running\":true"))
				},
			),
		}
	}

	testCase.Run(t)
}

func TestAttachDetachKeys(t *testing.T) {
	// In nerdctl the detach return code from the container after attach is 0, but in docker the return code is 1.
	// This behaviour is reported in https://github.com/containerd/nerdctl/issues/3571
	ex := 0
	if nerdtest.IsDocker() {
		ex = 1
	}

	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		cmd := helpers.Command("run", "--rm", "-it", "--detach-keys=ctrl-q", "--name", data.Identifier(), testutil.CommonImage)
		cmd.WithPseudoTTY()
		cmd.Feed(bytes.NewReader([]byte{17}))

		cmd.Run(&test.Expected{
			ExitCode: 0,
			Errors:   []error{errors.New("read detach keys")},
			Output: func(stdout string, info string, t *testing.T) {
				assert.Assert(t, strings.Contains(helpers.Capture("inspect", "--format", "json", data.Identifier()), "\"Running\":true"))
			},
		})
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// Run interactively and detach
		cmd := helpers.Command("attach", "--detach-keys=ctrl-a,ctrl-b", data.Identifier())
		cmd.WithPseudoTTY()
		cmd.Feed(strings.NewReader("echo mark${NON}mark\n"))
		cmd.WithFeeder(func() io.Reader {
			// Interestingly, and unlike with run, on attach, docker (like nerdctl) ALSO needs a pause so that the
			// container can read stdin before we detach
			time.Sleep(time.Second)
			// ctrl+p and ctrl+q (see https://en.wikipedia.org/wiki/C0_and_C1_control_codes)
			return bytes.NewReader([]byte{1, 2})
		})

		return cmd
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: ex,
			Errors:   []error{errors.New("read detach keys")},
			Output: expect.All(
				expect.Contains("markmark"),
				func(stdout string, info string, t *testing.T) {
					assert.Assert(t, strings.Contains(helpers.Capture("inspect", "--format", "json", data.Identifier()), "\"Running\":true"))
				},
			),
		}
	}

	testCase.Run(t)
}

// TestIssue3568 tests https://github.com/containerd/nerdctl/issues/3568
func TestAttachForAutoRemovedContainer(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Description = "Issue #3568 - A container should be deleted when detaching and attaching a container started with the --rm option."

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		cmd := helpers.Command("run", "--rm", "-it", "--detach-keys=ctrl-a,ctrl-b", "--name", data.Identifier(), testutil.CommonImage)
		cmd.WithPseudoTTY()
		// ctrl+a and ctrl+b (see https://en.wikipedia.org/wiki/C0_and_C1_control_codes)
		cmd.Feed(bytes.NewReader([]byte{1, 2}))

		cmd.Run(&test.Expected{
			ExitCode: 0,
			Errors:   []error{errors.New("read detach keys")},
			Output: func(stdout string, info string, t *testing.T) {
				assert.Assert(t, strings.Contains(helpers.Capture("inspect", "--format", "json", data.Identifier()), "\"Running\":true"), info)
			},
		})
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// Run interactively and detach
		cmd := helpers.Command("attach", data.Identifier())
		cmd.WithPseudoTTY()
		cmd.Feed(strings.NewReader("echo mark${NON}mark\nexit 42\n"))

		return cmd
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 42,
			Output: expect.All(
				expect.Contains("markmark"),
				func(stdout string, info string, t *testing.T) {
					assert.Assert(t, !strings.Contains(helpers.Capture("ps", "-a"), data.Identifier()))
				},
			),
		}
	}

	testCase.Run(t)
}
