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

package network

import (
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestNetworkPrune(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.Private

	testCase.SubTests = []*test.Case{
		{
			Description: "Prune does not collect started container network",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("network", "create", data.Identifier())
				helpers.Ensure("run", "-d", "--net", data.Identifier(), "--name", data.Identifier(), testutil.NginxAlpineImage)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
				helpers.Anyhow("network", "rm", data.Identifier())
			},
			Command: test.Command("network", "prune", "-f"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.DoesNotContain(data.Identifier()),
				}
			},
		},
		{
			Description: "Prune does collect stopped container network",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				helpers.Ensure("network", "create", data.Identifier())
				helpers.Ensure("run", "-d", "--net", data.Identifier(), "--name", data.Identifier(), testutil.NginxAlpineImage)
				helpers.Ensure("stop", data.Identifier())
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier())
				helpers.Anyhow("network", "rm", data.Identifier())
			},
			Command: test.Command("network", "prune", "-f"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.Contains(data.Identifier()),
				}
			},
		},
	}

	testCase.Run(t)
}
