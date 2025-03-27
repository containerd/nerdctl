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

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

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
		flags := "-a"
		// Started container must be interactive - which is apparently the default for nerdctl, which does not support
		// the -i flag, while docker requires it explicitly
		if nerdtest.IsDocker() {
			flags += "i"
		}
		cmd := helpers.Command("start", flags, "--detach-keys=ctrl-a,ctrl-b", data.Identifier())
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
				func(stdout string, info string, t *testing.T) {
					assert.Assert(t, strings.Contains(helpers.Capture("inspect", "--format", "json", data.Identifier()), "\"Running\":true"))
				},
			),
		}
	}

	testCase.Run(t)
}
