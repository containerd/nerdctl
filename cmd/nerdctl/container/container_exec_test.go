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
	"runtime"
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

func TestExec(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rm", "-f", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("exec", data.Identifier(), "echo", "success")
		},
		Expected: test.Expects(0, nil, expect.Equals("success\n")),
	}
	testCase.Run(t)
}

func TestExecWithDoubleDash(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Require: require.Not(nerdtest.Docker),
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rm", "-f", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("exec", data.Identifier(), "--", "echo", "success")
		},
		Expected: test.Expects(0, nil, expect.Equals("success\n")),
	}
	testCase.Run(t)
}

func TestExecStdin(t *testing.T) {
	nerdtest.Setup()

	const testStr = "test-exec-stdin"
	testCase := &test.Case{
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rm", "-f", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			cmd := helpers.Command("exec", "-i", data.Identifier(), "cat")
			cmd.Feed(strings.NewReader(testStr))
			return cmd
		},
		Expected: test.Expects(0, nil, expect.Equals(testStr)),
	}
	testCase.Run(t)
}

// FYI: https://github.com/containerd/nerdctl/blob/e4b2b6da56555dc29ed66d0fd8e7094ff2bc002d/cmd/nerdctl/run_test.go#L177
func TestExecEnv(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Env: map[string]string{
			"CORGE":  "corge-value-in-host",
			"GARPLY": "garply-value-in-host",
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage, "sleep", nerdtest.Infinity)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rm", "-f", data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("exec",
				"--env", "FOO=foo1,foo2",
				"--env", "BAR=bar1 bar2",
				"--env", "BAZ=",
				"--env", "QUX", // not exported in OS
				"--env", "QUUX=quux1",
				"--env", "QUUX=quux2",
				"--env", "CORGE", // OS exported
				"--env", "GRAULT=grault_key=grault_value", // value contains `=` char
				"--env", "GARPLY=", // OS exported
				"--env", "WALDO=", // not exported in OS

				data.Identifier(), "env")
		},
		Expected: test.Expects(0, nil, func(stdout string, t tig.T) {
			assert.Assert(t, strings.Contains(stdout, "\nFOO=foo1,foo2\n"), "got bad FOO")
			assert.Assert(t, strings.Contains(stdout, "\nBAR=bar1 bar2\n"), "got bad BAR")
			if runtime.GOOS != "windows" {
				assert.Assert(t, strings.Contains(stdout, "\nBAZ=\n"), "got bad BAZ")
			}
			assert.Assert(t, !strings.Contains(stdout, "QUX"), "got bad QUX (should not be set)")
			assert.Assert(t, strings.Contains(stdout, "\nQUUX=quux2\n"), "got bad QUUX")
			assert.Assert(t, strings.Contains(stdout, "\nCORGE=corge-value-in-host\n"), "got bad CORGE")
			assert.Assert(t, strings.Contains(stdout, "\nGRAULT=grault_key=grault_value\n"), "got bad GRAULT")
			if runtime.GOOS != "windows" {
				assert.Assert(t, strings.Contains(stdout, "\nGARPLY=\n"), "got bad GARPLY")
				assert.Assert(t, strings.Contains(stdout, "\nWALDO=\n"), "got bad WALDO")
			}
		}),
	}
	testCase.Run(t)
}
