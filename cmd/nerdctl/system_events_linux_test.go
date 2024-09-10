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

package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func testEventFilter(t *testing.T, args ...string) string {
	t.Parallel()
	base := testutil.NewBase(t)
	testContainerName := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", testContainerName).Run()

	fullArgs := []string{"events", "--filter"}
	fullArgs = append(fullArgs, args...)
	fullArgs = append(fullArgs,
		"--format",
		"json",
	)

	eventsCmd := base.Cmd(fullArgs...).Start()
	base.Cmd("run", "--rm", testutil.CommonImage).Start()
	time.Sleep(3 * time.Second)
	return eventsCmd.Stdout()
}

func TestEventFilters(t *testing.T) {

	type testCase struct {
		name       string
		args       []string
		nerdctlOut string
		dockerOut  string
		dockerSkip bool
	}
	testCases := []testCase{
		{
			name:       "CapitializedFilter",
			args:       []string{"event=START"},
			nerdctlOut: "\"Status\":\"start\"",
			dockerOut:  "\"status\":\"start\"",
			dockerSkip: true,
		},
		{
			name:       "StartEventFilter",
			args:       []string{"event=start"},
			nerdctlOut: "\"Status\":\"start\"",
			dockerOut:  "\"status\":\"start\"",
			dockerSkip: false,
		},
		{
			name:       "UnsupportedEventFilter",
			args:       []string{"event=unknown"},
			nerdctlOut: "\"Status\":\"unknown\"",
			dockerSkip: true,
		},
		{
			name:       "StatusFilter",
			args:       []string{"status=start"},
			nerdctlOut: "\"Status\":\"start\"",
			dockerOut:  "\"status\":\"start\"",
			dockerSkip: false,
		},
		{
			name:       "UnsupportedStatusFilter",
			args:       []string{"status=unknown"},
			nerdctlOut: "\"Status\":\"unknown\"",
			dockerSkip: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			actualOut := testEventFilter(t, tc.args...)
			errorMsg := fmt.Sprintf("%s failed;\nActual Filter Result: '%s'", tc.name, actualOut)

			isDocker := testutil.GetTarget() == testutil.Docker
			if isDocker && tc.dockerSkip {
				t.Skip("test is incompatible with Docker")
			}

			if isDocker {
				assert.Equal(t, true, strings.Contains(actualOut, tc.dockerOut), errorMsg)
			} else {
				assert.Equal(t, true, strings.Contains(actualOut, tc.nerdctlOut), errorMsg)
			}
		})
	}
}
