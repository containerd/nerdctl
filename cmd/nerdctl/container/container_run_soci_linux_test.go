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
	"strconv"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestRunSoci(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.All(
		require.Not(nerdtest.Docker),
		nerdtest.Soci,
	)

	// Tests relying on the output of "mount" cannot be run in parallel obviously
	testCase.NoParallel = true

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Custom("mount").Run(&test.Expected{
			ExitCode: 0,
			Output: func(stdout, info string, t *testing.T) {
				data.Labels().Set("beforeCount", strconv.Itoa(strings.Count(stdout, "fuse.rawBridge")))
			},
		})
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rmi", "-f", testutil.FfmpegSociImage)
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("--snapshotter=soci", "run", "--rm", testutil.FfmpegSociImage)
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			Output: func(stdout, info string, t *testing.T) {
				var afterCount int
				beforeCount, _ := strconv.Atoi(data.Labels().Get("beforeCount"))

				helpers.Custom("mount").Run(&test.Expected{
					Output: func(stdout, info string, t *testing.T) {
						afterCount = strings.Count(stdout, "fuse.rawBridge")
					},
				})

				assert.Equal(t, 11, afterCount-beforeCount, "expected the number of fuse.rawBridge")
			},
		}
	}

	testCase.Run(t)
}
