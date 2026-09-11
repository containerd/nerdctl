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
	"sync"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

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
			Output: func(stdout string, t tig.T) {
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
				func(stdout string, t tig.T) {
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
			Output: func(stdout string, t tig.T) {
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
				func(stdout string, t tig.T) {
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
			Output: func(stdout string, t tig.T) {
				assert.Assert(t, strings.Contains(helpers.Capture("inspect", "--format", "json", data.Identifier()), "\"Running\":true"))
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
				func(stdout string, t tig.T) {
					assert.Assert(t, !strings.Contains(helpers.Capture("ps", "-a"), data.Identifier()))
				},
			),
		}
	}

	testCase.Run(t)
}

func TestAttachNoStdin(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		cmd := helpers.Command("run", "-it", "--detach-keys=ctrl-p,ctrl-q", "--name", data.Identifier(),
			testutil.CommonImage, "sleep", "5")
		cmd.WithPseudoTTY()
		cmd.Feed(bytes.NewReader([]byte{16, 17})) // Ctrl-p, Ctrl-q to detach (https://en.wikipedia.org/wiki/C0_and_C1_control_codes)
		cmd.Run(&test.Expected{
			ExitCode: 0,
			Output: func(stdout string, t tig.T) {
				assert.Assert(t, strings.Contains(helpers.Capture("inspect", "--format", "{{.State.Running}}", data.Identifier()), "true"))
			},
		})
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		cmd := helpers.Command("attach", "--no-stdin", data.Identifier())
		cmd.WithPseudoTTY()
		cmd.Feed(strings.NewReader("should-not-appear\n"))
		cmd.Feed(bytes.NewReader([]byte{16, 17}))
		return cmd
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0, // Since it's a normal exit and not detach.
			Output: func(stdout string, t tig.T) {
				logs := helpers.Capture("logs", data.Identifier())
				assert.Assert(t, !strings.Contains(logs, "should-not-appear"))
			},
		}
	}

	testCase.Run(t)
}

// TestAttachMultipleSessions is the reproduction of
// https://github.com/containerd/nerdctl/issues/4374: with two sessions attached
// to the same container, a FIFO hands each chunk of output to exactly one
// reader, so one terminal appears to freeze. Both sessions must see everything.
func TestAttachMultipleSessions(t *testing.T) {
	// Detaching returns 0 on nerdctl and 1 on docker, as in TestAttach.
	// https://github.com/containerd/nerdctl/issues/3571
	ex := 0
	if nerdtest.IsDocker() {
		ex = 1
	}

	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		cmd := helpers.Command("run", "-d", "-it", "--name", data.Identifier(), testutil.CommonImage)
		cmd.Run(&test.Expected{ExitCode: 0})
		// Give the container time to reach its shell prompt.
		time.Sleep(time.Second)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		// A second session that stays attached for the whole test. It has to be
		// joined before this function returns: running it in a goroutine that
		// outlives the test would let the framework tear the container down
		// before the assertion runs, and would touch testing.T after the test
		// has finished.
		var wg sync.WaitGroup
		var observed string

		observer := helpers.Command("attach", data.Identifier())
		observer.WithPseudoTTY()
		observer.WithFeeder(func() io.Reader {
			time.Sleep(4 * time.Second)
			// ctrl+p ctrl+q
			return bytes.NewReader([]byte{16, 17})
		})

		wg.Add(1)
		go func() {
			defer wg.Done()
			// The Output callback only records what this session saw. The
			// assertion itself is done from the main goroutine in Cleanup,
			// because calling t.Fatal from a non-test goroutine is not valid.
			observer.Run(&test.Expected{
				ExitCode: ex,
				Output: func(stdout string, _ tig.T) {
					observed = stdout
				},
			})
		}()

		time.Sleep(time.Second)

		cmd := helpers.Command("attach", data.Identifier())
		cmd.WithPseudoTTY()
		cmd.Feed(strings.NewReader("echo mark${NON}mark\n"))
		cmd.WithFeeder(func() io.Reader {
			time.Sleep(2 * time.Second)
			// ctrl+p ctrl+q
			return bytes.NewReader([]byte{16, 17})
		})

		// Joining here is what makes the assertion meaningful: without it the
		// framework could tear the container down, and the test could finish,
		// before the observer ever produced any output.
		t.Cleanup(func() {
			wg.Wait()
			// This is the reproduction of #4374: before the broker, the two
			// sessions split the container's output and at most one of them
			// contained the marker.
			assert.Assert(t, strings.Contains(observed, "markmark"),
				"the second session did not see the container's output: %q", observed)
		})

		return cmd
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: ex,
			Output: expect.All(
				expect.Contains("markmark"),
				func(stdout string, t tig.T) {
					assert.Assert(t, strings.Contains(helpers.Capture("inspect", "--format", "json", data.Identifier()), "\"Running\":true"))
				},
			),
		}
	}

	testCase.Run(t)
}

// TestAttachToDetachedContainer covers a container started with -d. Before the
// stdio broker existed there was no stdin FIFO for such a container at all, so
// attaching to it could not work.
func TestAttachToDetachedContainer(t *testing.T) {
	// Detaching from an attach session returns 0 on nerdctl and 1 on docker.
	// https://github.com/containerd/nerdctl/issues/3571
	ex := 0
	if nerdtest.IsDocker() {
		ex = 1
	}

	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "-it", "--name", data.Identifier(), testutil.CommonImage)
		time.Sleep(time.Second)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		cmd := helpers.Command("attach", data.Identifier())
		cmd.WithPseudoTTY()
		cmd.Feed(strings.NewReader("echo detached${NON}marker\n"))
		cmd.WithFeeder(func() io.Reader {
			time.Sleep(2 * time.Second)
			// ctrl+p ctrl+q
			return bytes.NewReader([]byte{16, 17})
		})
		return cmd
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: ex,
			Output:   expect.Contains("detachedmarker"),
		}
	}

	testCase.Run(t)
}

// TestAttachAfterDetachKeepsContainerResponsive covers the case where the
// foreground session leaves: the container must keep producing output for a
// session that attaches afterwards, rather than stalling on stdio nobody reads.
func TestAttachAfterDetachKeepsContainerResponsive(t *testing.T) {
	// As above: detaching from an attach session returns 0 on nerdctl, 1 on
	// docker. The run+detach in Setup returns 0 on both.
	ex := 0
	if nerdtest.IsDocker() {
		ex = 1
	}

	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		cmd := helpers.Command("run", "-it", "--name", data.Identifier(), testutil.CommonImage)
		cmd.WithPseudoTTY()
		cmd.Feed(bytes.NewReader([]byte{16, 17}))
		cmd.Run(&test.Expected{ExitCode: 0})
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		cmd := helpers.Command("attach", data.Identifier())
		cmd.WithPseudoTTY()
		cmd.Feed(strings.NewReader("echo after${NON}detach\n"))
		cmd.WithFeeder(func() io.Reader {
			time.Sleep(2 * time.Second)
			// ctrl+p ctrl+q
			return bytes.NewReader([]byte{16, 17})
		})
		return cmd
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: ex,
			Output:   expect.Contains("afterdetach"),
		}
	}

	testCase.Run(t)
}

// TestAttachToDetachedContainerWithoutTTY covers `nerdctl run -d` without -t.
// Its output also goes to the logging process, through cio.LogURI, so the same
// socket serves it. It has no stdin: the shim does not wire one for a
// non-terminal container whose stdout is a binary URI.
func TestAttachToDetachedContainerWithoutTTY(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// The leading sleep gives the attach below time to connect. There is no
		// replay buffer, so anything printed before it connects is gone.
		helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage,
			"sh", "-c", "sleep 2; for i in 1 2 3; do echo tick; sleep 1; done")
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("attach", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		// attach returns when the container exits, with its exit code.
		return &test.Expected{
			ExitCode: 0,
			Output:   expect.Contains("tick"),
		}
	}

	testCase.Run(t)
}
