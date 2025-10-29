//go:build linux

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

package checkpoint

import (
	"errors"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestCheckpointListErrors(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.All(
		require.Not(nerdtest.Rootless),
		// Docker version 28.x has a known regression that breaks Checkpoint/Restore functionality.
		// The issue is tracked in the moby/moby project as https://github.com/moby/moby/issues/50750.
		require.Not(nerdtest.Docker),
	)

	testCase.SubTests = []*test.Case{
		{
			Description: "too-few-arguments",
			Command:     test.Command("checkpoint", "list"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{ExitCode: 1}
			},
		},
		{
			Description: "too-many-arguments",
			Command:     test.Command("checkpoint", "list", "too", "many", "arguments"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{ExitCode: 1}
			},
		},
		{
			Description: "invalid-container-id",
			Command:     test.Command("checkpoint", "list", "no-such-container"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New("error list checkpoint for container: no-such-container")},
				}
			},
		},
	}

	testCase.Run(t)
}

func TestCheckpointList(t *testing.T) {
	const checkpointName = "checkpoint-list"

	testCase := nerdtest.Setup()
	testCase.Require = require.All(
		require.Not(nerdtest.Rootless),
		// Docker version 28.x has a known regression that breaks Checkpoint/Restore functionality.
		// The issue is tracked in the moby/moby project as https://github.com/moby/moby/issues/50750.
		require.Not(nerdtest.Docker),
	)
	testCase.NoParallel = true
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", "infinity")
		helpers.Ensure("checkpoint", "create", data.Identifier(), checkpointName)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("checkpoint", "list", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			// First line is header, second should include the checkpoint name
			Output: expect.Contains("CHECKPOINT NAME\n" + checkpointName + "\n"),
		}
	}

	testCase.Run(t)
}
