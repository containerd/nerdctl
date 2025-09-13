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

func TestCheckpointCreateErrors(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Rootless)
	testCase.SubTests = []*test.Case{
		{
			Description: "too-few-arguments",
			Command:     test.Command("checkpoint", "create", "too-few-arguments"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
				}
			},
		},
		{
			Description: "too-many-arguments",
			Command:     test.Command("checkpoint", "create", "too", "many", "arguments"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
				}
			},
		},
		{
			Description: "invalid-container-id",
			Command:     test.Command("checkpoint", "create", "foo", "bar"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 1,
					Errors:   []error{errors.New("error creating checkpoint for container: foo")},
				}
			},
		},
	}

	testCase.Run(t)
}

func TestCheckpointCreate(t *testing.T) {
	const (
		checkpointName = "checkpoint-bar"
		checkpointDir  = "/dir/foo"
	)
	testCase := nerdtest.Setup()
	testCase.Require = require.Not(nerdtest.Rootless)
	testCase.SubTests = []*test.Case{
		{
			Description: "leave-running=true",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier("container-running"), testutil.CommonImage, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier("container-running"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("checkpoint", "create", "--leave-running", "--checkpoint-dir", checkpointDir, data.Identifier("container-running"), checkpointName+"running")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output:   expect.Equals(checkpointName + "running\n"),
				}
			},
		},
		{
			Description: "leave-running=false",
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("run", "-d", "--name", data.Identifier("container-exit"), testutil.CommonImage, "sleep", "infinity")
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier("container-exit"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("checkpoint", "create", "--checkpoint-dir", checkpointDir, data.Identifier("container-exit"), checkpointName+"exit")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Output:   expect.Equals(checkpointName + "exit\n"),
				}
			},
		},
	}

	testCase.Run(t)
}
