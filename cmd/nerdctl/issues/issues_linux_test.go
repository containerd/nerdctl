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

// Package issues is meant to document testing for complex scenarios type of issues that cannot simply be ascribed
// to a specific package.
package issues

import (
	"fmt"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testregistry"
)

func TestMain(m *testing.M) {
	testutil.M(m)
}

func TestIssue3425(t *testing.T) {
	nerdtest.Setup()

	var registry *testregistry.RegistryServer

	testCase := &test.Case{
		Description: "TestIssue3425",
		Setup: func(data test.Data, helpers test.Helpers) {
			base := testutil.NewBase(t)
			registry = testregistry.NewWithNoAuth(base, 0, false)
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			if registry != nil {
				registry.Cleanup(nil)
			}
		},
		SubTests: []*test.Case{
			{
				Description: "with tag",
				Require:     nerdtest.Private,
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("image", "pull", testutil.CommonImage)
					helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage)
					helpers.Ensure("image", "rm", "-f", testutil.CommonImage)
					helpers.Ensure("image", "pull", testutil.CommonImage)
					helpers.Ensure("tag", testutil.CommonImage, fmt.Sprintf("localhost:%d/%s", registry.Port, data.Identifier()))
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rm", "-f", data.Identifier())
					helpers.Anyhow("rmi", "-f", fmt.Sprintf("localhost:%d/%s", registry.Port, data.Identifier()))
				},
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.Command("push", fmt.Sprintf("localhost:%d/%s", registry.Port, data.Identifier()))
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "with commit",
				Require:     nerdtest.Private,
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("image", "pull", testutil.CommonImage)
					helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage, "touch", "/something")
					helpers.Ensure("image", "rm", "-f", testutil.CommonImage)
					helpers.Ensure("image", "pull", testutil.CommonImage)
					helpers.Ensure("commit", data.Identifier(), fmt.Sprintf("localhost:%d/%s", registry.Port, data.Identifier()))
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rm", "-f", data.Identifier())
					helpers.Anyhow("rmi", "-f", fmt.Sprintf("localhost:%d/%s", registry.Port, data.Identifier()))
				},
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.Command("push", fmt.Sprintf("localhost:%d/%s", registry.Port, data.Identifier()))
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "with save",
				Require:     nerdtest.Private,
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("image", "pull", testutil.CommonImage)
					helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage)
					helpers.Ensure("image", "rm", "-f", testutil.CommonImage)
					helpers.Ensure("image", "pull", testutil.CommonImage)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rm", "-f", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.Command("save", testutil.CommonImage)
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "with convert",
				Require: test.Require(
					nerdtest.Private,
					test.Not(test.Windows),
					test.Not(nerdtest.Docker),
				),
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("image", "pull", testutil.CommonImage)
					helpers.Ensure("run", "-d", "--name", data.Identifier(), testutil.CommonImage)
					helpers.Ensure("image", "rm", "-f", testutil.CommonImage)
					helpers.Ensure("image", "pull", testutil.CommonImage)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rm", "-f", data.Identifier())
					helpers.Anyhow("rmi", "-f", data.Identifier())
				},
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.Command("image", "convert", "--oci", "--estargz", testutil.CommonImage, data.Identifier())
				},
				Expected: test.Expects(0, nil, nil),
			},
		},
	}

	testCase.Run(t)
}
