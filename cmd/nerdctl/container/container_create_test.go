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

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestCreate(t *testing.T) {
	testCase := nerdtest.Setup()
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("create", "--name", data.Identifier("container"), testutil.CommonImage, "echo", "foo")
		data.Set("cID", data.Identifier("container"))
	}
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("container"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "ps -a",
			NoParallel:  true,
			Command:     test.Command("ps", "-a"),
			// FIXME: this might get a false positive if other tests have created a container
			Expected: test.Expects(0, nil, test.Contains("Created")),
		},
		{
			Description: "start",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("start", data.Get("cID"))
			},
			Expected: test.Expects(0, nil, nil),
		},
		{
			Description: "logs",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", data.Get("cID"))
			},
			Expected: test.Expects(0, nil, test.Contains("foo")),
		},
	}

	testCase.Run(t)
}

func TestCreateHyperVContainer(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = test.Windows

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("create", "--isolation", "hyperv", "--name", data.Identifier("container"), testutil.CommonImage, "echo", "foo")
		data.Set("cID", data.Identifier("container"))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier("container"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "ps -a",
			NoParallel:  true,
			Command:     test.Command("ps", "-a"),
			// FIXME: this might get a false positive if other tests have created a container
			Expected: test.Expects(0, nil, test.Contains("Created")),
		},
		{
			Description: "start",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("start", data.Get("cID"))
				// hyperv containers take a few seconds to fire up, the test would fail without the sleep
				// EnsureContainerStarted does not work
				time.Sleep(10 * time.Second)
			},
		},
		{
			Description: "logs",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("logs", data.Get("cID"))
			},
			Expected: test.Expects(0, nil, test.Contains("foo")),
		},
	}

	testCase.Run(t)
}
