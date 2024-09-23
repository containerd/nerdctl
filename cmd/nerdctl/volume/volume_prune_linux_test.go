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
	"strings"
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestVolumePrune(t *testing.T) {
	nerdtest.Setup()

	var setup = func(data test.Data, helpers test.Helpers) {
		anonIDBusy := strings.TrimSpace(helpers.Capture("volume", "create"))
		anonIDDangling := strings.TrimSpace(helpers.Capture("volume", "create"))

		namedBusy := data.Identifier() + "-busy"
		namedDangling := data.Identifier() + "-free"

		helpers.Ensure("volume", "create", namedBusy)
		helpers.Ensure("volume", "create", namedDangling)
		helpers.Ensure("run", "--name", data.Identifier(),
			"-v", namedBusy+":/whatever",
			"-v", anonIDBusy+":/other", testutil.CommonImage)

		data.Set("anonIDBusy", anonIDBusy)
		data.Set("anonIDDangling", anonIDDangling)
		data.Set("namedBusy", namedBusy)
		data.Set("namedDangling", namedDangling)
	}

	var cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
		helpers.Anyhow("rm", "-f", data.Get("anonIDBusy"))
		helpers.Anyhow("rm", "-f", data.Get("anonIDDangling"))
		helpers.Anyhow("rm", "-f", data.Get("namedBusy"))
		helpers.Anyhow("rm", "-f", data.Get("namedDangling"))
	}

	// This set must be marked as private, since we cannot prune without interacting with other tests.
	testGroup := &test.Group{
		{
			Description: "prune anonymous only",
			Require:     nerdtest.Private,
			Command:     test.RunCommand("volume", "prune", "-f"),
			Setup:       setup,
			Cleanup:     cleanup,
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.All(
						test.DoesNotContain(data.Get("anonIDBusy")),
						test.Contains(data.Get("anonIDDangling")),
						test.DoesNotContain(data.Get("namedBusy")),
						test.DoesNotContain(data.Get("namedDangling")),
						func(stdout string, info string, t *testing.T) {
							helpers.Ensure("volume", "inspect", data.Get("anonIDBusy"))
							helpers.Fail("volume", "inspect", data.Get("anonIDDangling"))
							helpers.Ensure("volume", "inspect", data.Get("namedBusy"))
							helpers.Ensure("volume", "inspect", data.Get("namedDangling"))
						},
					),
				}
			},
		},
		{
			Description: "prune all",
			Require:     nerdtest.Private,
			Command:     test.RunCommand("volume", "prune", "-f", "--all"),
			Setup:       setup,
			Cleanup:     cleanup,
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.All(
						test.DoesNotContain(data.Get("anonIDBusy")),
						test.Contains(data.Get("anonIDDangling")),
						test.DoesNotContain(data.Get("namedBusy")),
						test.Contains(data.Get("namedDangling")),
						func(stdout string, info string, t *testing.T) {
							helpers.Ensure("volume", "inspect", data.Get("anonIDBusy"))
							helpers.Fail("volume", "inspect", data.Get("anonIDDangling"))
							helpers.Ensure("volume", "inspect", data.Get("namedBusy"))
							helpers.Fail("volume", "inspect", data.Get("namedDangling"))
						},
					),
				}
			},
		},
	}

	testGroup.Run(t)
}
