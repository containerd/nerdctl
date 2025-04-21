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

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func TestVolumePrune(t *testing.T) {
	var setup = func(data test.Data, helpers test.Helpers) {
		anonIDBusy := strings.TrimSpace(helpers.Capture("volume", "create"))
		anonIDDangling := strings.TrimSpace(helpers.Capture("volume", "create"))

		namedBusy := data.Identifier("busy")
		namedDangling := data.Identifier("free")

		helpers.Ensure("volume", "create", namedBusy)
		helpers.Ensure("volume", "create", namedDangling)
		helpers.Ensure("run", "--name", data.Identifier(),
			"-v", namedBusy+":/namedbusyvolume",
			"-v", anonIDBusy+":/anonbusyvolume", testutil.CommonImage)

		data.Labels().Set("anonIDBusy", anonIDBusy)
		data.Labels().Set("anonIDDangling", anonIDDangling)
		data.Labels().Set("namedBusy", namedBusy)
		data.Labels().Set("namedDangling", namedDangling)
	}

	var cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
		helpers.Anyhow("volume", "rm", "-f", data.Labels().Get("anonIDBusy"))
		helpers.Anyhow("volume", "rm", "-f", data.Labels().Get("anonIDDangling"))
		helpers.Anyhow("volume", "rm", "-f", data.Labels().Get("namedBusy"))
		helpers.Anyhow("volume", "rm", "-f", data.Labels().Get("namedDangling"))
	}

	testCase := nerdtest.Setup()
	// This set must be marked as private, since we cannot prune without interacting with other tests.
	testCase.Require = nerdtest.Private
	// Furthermore, these two subtests cannot be run in parallel
	testCase.SubTests = []*test.Case{
		{
			Description: "prune anonymous only",
			NoParallel:  true,
			Setup:       setup,
			Cleanup:     cleanup,
			Command:     test.Command("volume", "prune", "-f"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.All(
						expect.Contains(data.Labels().Get("anonIDDangling")),
						expect.DoesNotContain(
							data.Labels().Get("anonIDBusy"),
							data.Labels().Get("namedBusy"),
							data.Labels().Get("namedDangling"),
						),
						func(stdout string, info string, t *testing.T) {
							helpers.Ensure("volume", "inspect", data.Labels().Get("anonIDBusy"))
							helpers.Fail("volume", "inspect", data.Labels().Get("anonIDDangling"))
							helpers.Ensure("volume", "inspect", data.Labels().Get("namedBusy"))
							helpers.Ensure("volume", "inspect", data.Labels().Get("namedDangling"))
						},
					),
				}
			},
		},
		{
			Description: "prune all",
			NoParallel:  true,
			Setup:       setup,
			Cleanup:     cleanup,
			Command:     test.Command("volume", "prune", "-f", "--all"),
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.All(
						expect.DoesNotContain(data.Labels().Get("anonIDBusy"), data.Labels().Get("namedBusy")),
						expect.Contains(data.Labels().Get("anonIDDangling"), data.Labels().Get("namedDangling")),
						func(stdout string, info string, t *testing.T) {
							helpers.Ensure("volume", "inspect", data.Labels().Get("anonIDBusy"))
							helpers.Fail("volume", "inspect", data.Labels().Get("anonIDDangling"))
							helpers.Ensure("volume", "inspect", data.Labels().Get("namedBusy"))
							helpers.Fail("volume", "inspect", data.Labels().Get("namedDangling"))
						},
					),
				}
			},
		},
	}

	testCase.Run(t)
}
