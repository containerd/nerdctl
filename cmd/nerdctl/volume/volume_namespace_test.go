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

package volume

import (
	"testing"

	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestVolumeNamespace(t *testing.T) {
	testCase := nerdtest.Setup()

	// Docker does not support namespaces
	testCase.Require = test.Not(nerdtest.Docker)

	// Create a volume in a different namespace
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Set("root_namespace", data.Identifier())
		data.Set("root_volume", data.Identifier())
		helpers.Ensure("--namespace", data.Identifier(), "volume", "create", data.Identifier())
	}

	// Cleanup once done
	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if data.Get("root_namespace") != "" {
			helpers.Anyhow("--namespace", data.Identifier(), "volume", "remove", data.Identifier())
			helpers.Anyhow("namespace", "remove", data.Identifier())
		}
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "inspect another namespace volume should fail",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "inspect", data.Get("root_volume"))
			},
			Expected: test.Expects(1, []error{
				errdefs.ErrNotFound,
			}, nil),
		},
		{
			Description: "removing another namespace volume should fail",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "remove", data.Get("root_volume"))
			},
			Expected: test.Expects(1, []error{
				errdefs.ErrNotFound,
			}, nil),
		},
		{
			Description: "prune should leave another namespace volume untouched",
			// Make it private so that we do not interact with other tests in the main namespace
			Require: nerdtest.Private,
			Command: test.Command("volume", "prune", "-a", "-f"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.All(
						test.DoesNotContain(data.Get("root_volume")),
						func(stdout string, info string, t *testing.T) {
							helpers.Ensure("--namespace", data.Get("root_namespace"), "volume", "inspect", data.Get("root_volume"))
						},
					),
				}
			},
		},
		{
			Description: "create with the same name should work, then delete it",
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("volume", "create", data.Get("root_volume"))
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("volume", "rm", data.Get("root_volume"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, info string, t *testing.T) {
						helpers.Ensure("volume", "inspect", data.Get("root_volume"))
						helpers.Ensure("volume", "rm", data.Get("root_volume"))
						helpers.Ensure("--namespace", data.Get("root_namespace"), "volume", "inspect", data.Get("root_volume"))
					},
				}
			},
		},
	}

	testCase.Run(t)
}
