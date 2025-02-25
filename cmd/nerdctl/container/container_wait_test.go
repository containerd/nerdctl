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
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestWait(t *testing.T) {
	testCase := nerdtest.Setup()

	start := time.Now()

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("1"), data.Identifier("2"), data.Identifier("3"))
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		t.Log("in setup", time.Since(start))
		helpers.Ensure("pull", testutil.CommonImage)
		t.Log("pull done", time.Since(start))

		helpers.Ensure("run", "-d", "--pull", "never", "--name", data.Identifier("1"), testutil.CommonImage)
		t.Log("run 1", time.Since(start))
		helpers.Ensure("run", "-d", "--pull", "never", "--name", data.Identifier("2"), testutil.CommonImage, "sleep", "1")
		t.Log("run 2", time.Since(start))
		helpers.Ensure("run", "-d", "--pull", "never", "--name", data.Identifier("3"), testutil.CommonImage, "sh", "-euxc", "sleep 5; exit 123")
		t.Log("run 3", time.Since(start))
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		t.Log("in command", time.Since(start))
		return helpers.Command("wait", data.Identifier("1"), data.Identifier("2"), data.Identifier("3"))
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		t.Log("in expected", time.Since(start))
		return &test.Expected{
			ExitCode: 0,
			Output: func(stdout string, info string, t *testing.T) {
				t.Log("in output", time.Since(start))
				assert.Assert(t, false, "forcing stop")
			},
		}
	}

	testCase.Run(t)
}
