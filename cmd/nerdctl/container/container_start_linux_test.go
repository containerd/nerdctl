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
	"strconv"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestStartDetachKeys(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		cmd := helpers.Command("run", "-it", "--name", data.Identifier(), testutil.CommonImage)
		cmd.WithPseudoTTY()
		cmd.Feed(strings.NewReader("exit\n"))
		cmd.Run(&test.Expected{
			ExitCode: 0,
		})
		assert.Assert(t,
			strings.Contains(helpers.Capture("inspect", "--format", "json", data.Identifier()), "\"Running\":false"),
		)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		cmd := helpers.Command("start", "-ai", "--detach-keys=ctrl-a,ctrl-b", data.Identifier())
		cmd.WithPseudoTTY()
		cmd.WithFeeder(func() io.Reader {
			// ctrl+a and ctrl+b (see https://en.wikipedia.org/wiki/C0_and_C1_control_codes)
			return bytes.NewReader([]byte{1, 2})
		})

		return cmd
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Errors:   []error{errors.New("detach keys")},
			Output: expect.All(
				func(stdout string, t tig.T) {
					assert.Assert(t, strings.Contains(helpers.Capture("inspect", "--format", "json", data.Identifier()), "\"Running\":true"))
				},
			),
		}
	}

	testCase.Run(t)
}

func TestStartWithCheckpoint(t *testing.T) {

	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Rootless)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// Use an in-memory tmpfs to model in-memory state without introducing extra processes
		// Single PID 1 shell: continuously increment a counter and write to /state/counter (tmpfs)
		helpers.Ensure("run", "-d", "--name", data.Identifier(), "--tmpfs", "/state", testutil.CommonImage,
			"sh", "-c", `i=0; while true; do i=$((i+1)); printf "%d\n" "$i" >/state/counter; sleep 0.2; done`)
		// Give some time for the counter to increase before checkpoint to validate continuity after restore
		time.Sleep(1 * time.Second)
		helpers.Ensure("checkpoint", "create", data.Identifier(), data.Identifier()+"-checkpoint")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("start", "--checkpoint", data.Identifier()+"-checkpoint", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Output: expect.All(
				func(_ string, t tig.T) {
					// Validate in-memory state continuity via tmpfs: counter should not reset and must keep increasing
					// Short delay to allow the container to resume; if the counter had reset to 0, it could not reach >5 this fast
					time.Sleep(200 * time.Millisecond)
					c1Str := strings.TrimSpace(helpers.Capture("exec", data.Identifier(), "cat", "/state/counter"))
					var parseErrs []error
					c1, err1 := strconv.Atoi(c1Str)
					if err1 != nil {
						parseErrs = append(parseErrs, err1)
					}
					assert.Assert(t, len(parseErrs) == 0, "failed to parse counter values: %v", parseErrs)
					assert.Assert(t, c1 > 5, "tmpfs in-memory counter seems reset or too small: %d", c1)
				},
			),
		}
	}

	testCase.Run(t)
}
