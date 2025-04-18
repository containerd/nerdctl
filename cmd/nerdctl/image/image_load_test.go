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

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestLoadStdinFromPipe(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Description: "TestLoadStdinFromPipe",
		Require:     require.Linux,
		Setup: func(data test.Data, helpers test.Helpers) {
			identifier := data.Identifier()
			helpers.Ensure("pull", "--quiet", testutil.CommonImage)
			helpers.Ensure("tag", testutil.CommonImage, identifier)
			helpers.Ensure("save", identifier, "-o", filepath.Join(data.Temp().Path(), "common.tar"))
			helpers.Ensure("rmi", "-f", identifier)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			cmd := helpers.Command("load")
			reader, err := os.Open(filepath.Join(data.Temp().Path(), "common.tar"))
			assert.NilError(t, err, "failed to open common.tar")
			cmd.Feed(reader)
			return cmd
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			identifier := data.Identifier()
			return &test.Expected{
				Output: expect.All(
					expect.Contains(fmt.Sprintf("Loaded image: %s:latest", identifier)),
					func(stdout string, info string, t *testing.T) {
						assert.Assert(t, strings.Contains(helpers.Capture("images"), identifier))
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
		Require:     require.Linux,
		Command:     test.Command("load"),
		Expected:    test.Expects(1, nil, nil),
	}

	testCase.Run(t)
}

func TestLoadQuiet(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Description: "TestLoadQuiet",
		Setup: func(data test.Data, helpers test.Helpers) {
			identifier := data.Identifier()
			helpers.Ensure("pull", "--quiet", testutil.CommonImage)
			helpers.Ensure("tag", testutil.CommonImage, identifier)
			helpers.Ensure("save", identifier, "-o", filepath.Join(data.Temp().Path(), "common.tar"))
			helpers.Ensure("rmi", "-f", identifier)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rmi", "-f", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("load", "--quiet", "--input", filepath.Join(data.Temp().Path(), "common.tar"))
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				Output: expect.All(
					expect.Contains(fmt.Sprintf("Loaded image: %s:latest", data.Identifier())),
					expect.DoesNotContain("Loading layer"),
				),
			}
		},
	}

	testCase.Run(t)
}
