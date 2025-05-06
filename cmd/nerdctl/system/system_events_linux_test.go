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

package system

import (
	"testing"
	"time"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func testEventFilterExecutor(data test.Data, helpers test.Helpers) test.TestableCommand {
	cmd := helpers.Command("events", "--filter", data.Labels().Get("filter"), "--format", "json")
	// 3 seconds is too short on slow rig (EL8)
	cmd.WithTimeout(10 * time.Second)
	cmd.Background()
	helpers.Ensure("run", "--rm", testutil.CommonImage)
	return cmd
}

func TestEventFilters(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "CapitalizedFilter",
			Require:     require.Not(nerdtest.Docker),
			Command:     testEventFilterExecutor,
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeTimeout,
					Output:   expect.Contains(data.Labels().Get("output")),
				}
			},
			Data: test.WithLabels(map[string]string{
				"filter": "event=START",
				"output": "\"Status\":\"start\"",
			}),
		},
		{
			Description: "StartEventFilter",
			Command:     testEventFilterExecutor,
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeTimeout,
					Output:   expect.Contains(data.Labels().Get("output")),
				}
			},
			Data: test.WithLabels(map[string]string{
				"filter": "event=start",
				"output": "tatus\":\"start\"",
			}),
		},
		{
			Description: "UnsupportedEventFilter",
			Require:     require.Not(nerdtest.Docker),
			Command:     testEventFilterExecutor,
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeTimeout,
					Output:   expect.Contains(data.Labels().Get("output")),
				}
			},
			Data: test.WithLabels(map[string]string{
				"filter": "event=unknown",
				"output": "\"Status\":\"unknown\"",
			}),
		},
		{
			Description: "StatusFilter",
			Command:     testEventFilterExecutor,
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeTimeout,
					Output:   expect.Contains(data.Labels().Get("output")),
				}
			},
			Data: test.WithLabels(map[string]string{
				"filter": "status=start",
				"output": "tatus\":\"start\"",
			}),
		},
		{
			Description: "UnsupportedStatusFilter",
			Require:     require.Not(nerdtest.Docker),
			Command:     testEventFilterExecutor,
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: expect.ExitCodeTimeout,
					Output:   expect.Contains(data.Labels().Get("output")),
				}
			},
			Data: test.WithLabels(map[string]string{
				"filter": "status=unknown",
				"output": "\"Status\":\"unknown\"",
			}),
		},
	}

	testCase.Run(t)
}
