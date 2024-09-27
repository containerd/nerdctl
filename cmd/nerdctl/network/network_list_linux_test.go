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
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestNetworkLsFilter(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Description: "Test network list",
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Set("identifier", data.Identifier())
			data.Set("label", data.Identifier()+"=label-1")
			data.Set("netID1", helpers.Capture("network", "create", "--label="+data.Get("label"), data.Identifier()+"-1"))
			data.Set("netID2", helpers.Capture("network", "create", data.Identifier()+"-2"))
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("network", "rm", data.Identifier()+"-1")
			helpers.Anyhow("network", "rm", data.Identifier()+"-2")
		},
		SubTests: []*test.Case{
			{
				Description: "filter label",
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.Command("network", "ls", "--quiet", "--filter", "label="+data.Get("label"))
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							var lines = strings.Split(strings.TrimSpace(stdout), "\n")
							assert.Assert(t, len(lines) >= 1, info)
							netNames := map[string]struct{}{
								data.Get("netID1")[:12]: {},
							}

							for _, name := range lines {
								_, ok := netNames[name]
								assert.Assert(t, ok, info)
							}
						},
					}
				},
			},
			{
				Description: "filter name",
				Command: func(data test.Data, helpers test.Helpers) test.Command {
					return helpers.Command("network", "ls", "--quiet", "--filter", "name="+data.Get("identifier")+"-2")
				},
				Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
					return &test.Expected{
						Output: func(stdout string, info string, t *testing.T) {
							var lines = strings.Split(strings.TrimSpace(stdout), "\n")
							assert.Assert(t, len(lines) >= 1, info)
							netNames := map[string]struct{}{
								data.Get("netID2")[:12]: {},
							}

							for _, name := range lines {
								_, ok := netNames[name]
								assert.Assert(t, ok, info)
							}
						},
					}
				},
			},
		},
	}

	testCase.Run(t)
}
