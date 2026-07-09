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
	"errors"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

// TestRunDetachInteractiveWithTTY verifies that `run -t -d -i` is accepted and
// starts a detached container that keeps running, matching Docker. The TTY's pty
// is held by the shim, so the combination is valid.
func TestRunDetachInteractiveWithTTY(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "-t", "-d", "-i", "--name", data.Identifier(),
			testutil.CommonImage, "sleep", "infinity")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeSuccess,
			Output: func(stdout string, t tig.T) {
				assert.Assert(t, strings.Contains(
					helpers.Capture("inspect", "--format", "{{.State.Running}}", data.Identifier()), "true"))
			},
		}
	}

	testCase.Run(t)
}

// TestRunDetachInteractiveWithoutTTYFails verifies that `run -d -i` without -t is
// rejected: being daemonless, nerdctl has no process to keep stdin open after
// detaching. Docker (daemon-backed) supports it, so this is nerdctl-only.
func TestRunDetachInteractiveWithoutTTYFails(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Docker)

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("run", "-d", "-i", "--name", data.Identifier(), testutil.CommonImage, "cat")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: expect.ExitCodeGenericFail,
			Errors:   []error{errors.New("can only be specified together with -t")},
		}
	}

	testCase.Run(t)
}
