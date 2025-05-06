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

package compose

import (
	"errors"
	"fmt"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

// https://github.com/containerd/nerdctl/issues/1942
func TestComposeUpDetailedError(t *testing.T) {
	dockerComposeYAML := fmt.Sprintf(`
services:
  foo:
    image: %s
    runtime: invalid
`, testutil.CommonImage)

	testCase := nerdtest.Setup()

	// "FIXME: test does not work on Windows yet (runtime \"io.containerd.runc.v2\" binary not installed \"containerd-shim-runc-v2.exe\": file does not exist)
	testCase.Require = require.Not(require.Windows)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Temp().Save(dockerComposeYAML, "compose.yaml")
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("compose", "-f", data.Temp().Path("compose.yaml"), "up", "-d")
	}

	testCase.Expected = test.Expects(
		1,
		[]error{errors.New(`invalid runtime name`)},
		nil,
	)

	testCase.Run(t)
}

// https://github.com/containerd/nerdctl/issues/1652
func TestComposeUpBindCreateHostPath(t *testing.T) {
	testCase := nerdtest.Setup()

	// `FIXME: no support for Windows path: (error: "volume target must be an absolute path, got \"/mnt\")`
	testCase.Require = require.Not(require.Windows)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		var dockerComposeYAML = fmt.Sprintf(`
services:
  test:
    image: %s
    command: sh -euxc "echo hi >/mnt/test"
    volumes:
      # tempdir/foo should be automatically created
      - %s:/mnt
`, testutil.CommonImage, data.Temp().Path("foo"))

		data.Temp().Save(dockerComposeYAML, "compose.yaml")
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("compose", "-f", data.Temp().Path("compose.yaml"), "up")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("compose", "-f", data.Temp().Path("compose.yaml"), "down")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			ExitCode: 0,
			Errors:   nil,
			Output: func(stdout, info string, t *testing.T) {
				assert.Equal(t, data.Temp().Load("foo", "test"), "hi\n")
			},
		}
	}

	testCase.Run(t)
}
