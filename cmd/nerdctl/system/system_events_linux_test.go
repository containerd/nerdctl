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

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func testEventFilterExecutor(data test.Data, helpers test.Helpers) test.Command {
	cmd := helpers.Command("events", "--filter", data.Get("filter"), "--format", "json")
	cmd.Background(1 * time.Second)
	helpers.Ensure("run", "--rm", testutil.CommonImage)
	return cmd
}

func TestEventFilters(t *testing.T) {
	nerdtest.Setup()

	testGroup := &test.Group{
		{
			Description: "CapitalizedFilter",
			Require:     test.Not(nerdtest.Docker),
			Command:     testEventFilterExecutor,
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.Contains(data.Get("output")),
				}
			},
			Data: test.WithData("filter", "event=START").
				Set("output", "\"Status\":\"start\""),
		},
		{
			Description: "StartEventFilter",
			Command:     testEventFilterExecutor,
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.Contains(data.Get("output")),
				}
			},
			Data: test.WithData("filter", "event=start").
				Set("output", "tatus\":\"start\""),
		},
		{
			Description: "UnsupportedEventFilter",
			Require:     test.Not(nerdtest.Docker),
			Command:     testEventFilterExecutor,
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.Contains(data.Get("output")),
				}
			},
			Data: test.WithData("filter", "event=unknown").
				Set("output", "\"Status\":\"unknown\""),
		},
		{
			Description: "StatusFilter",
			Command:     testEventFilterExecutor,
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.Contains(data.Get("output")),
				}
			},
			Data: test.WithData("filter", "status=start").
				Set("output", "tatus\":\"start\""),
		},
		{
			Description: "UnsupportedStatusFilter",
			Require:     test.Not(nerdtest.Docker),
			Command:     testEventFilterExecutor,
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: test.Contains(data.Get("output")),
				}
			},
			Data: test.WithData("filter", "status=unknown").
				Set("output", "\"Status\":\"unknown\""),
		},
	}

	testGroup.Run(t)
}
