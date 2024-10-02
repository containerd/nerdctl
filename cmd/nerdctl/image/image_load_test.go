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

package image

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestLoadStdinFromPipe(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Description: "TestLoadStdinFromPipe",
		Require:     test.Linux,
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("pull", testutil.CommonImage)
			helpers.Ensure("tag", testutil.CommonImage, data.Identifier())
			helpers.Ensure("save", data.Identifier(), "-o", filepath.Join(data.TempDir(), "common.tar"))
			helpers.Ensure("rmi", "-f", data.Identifier())
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.Command {
			cmd := helpers.Command("load")
			reader, err := os.Open(filepath.Join(data.TempDir(), "common.tar"))
			assert.NilError(t, err, "failed to open common.tar")
			cmd.WithStdin(reader)
			return cmd
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				Output: test.All(
					test.Contains(fmt.Sprintf("Loaded image: %s:latest", data.Identifier())),
					func(stdout string, info string, t *testing.T) {
						assert.Assert(t, strings.Contains(helpers.Capture("images"), data.Identifier()))
					},
				),
			}
		},
	}

	testCase.Run(t)
}

func TestLoadStdinEmpty(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Description: "TestLoadStdinEmpty",
		Require:     test.Linux,
		Command:     test.RunCommand("load"),
		Expected:    test.Expects(1, nil, nil),
	}

	testCase.Run(t)
}
