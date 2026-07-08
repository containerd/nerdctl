//go:build linux || windows

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
	"fmt"
	"io"
	"net"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
)

// Tests various port mapping argument combinations by starting an nginx container and
// verifying its connectivity and that its serves its index.html from the external
// host IP as well as through the loopback interface.
// `loopbackIsolationEnabled` indicates whether the test should expect connections between
// the loopback interface and external host interface to succeed or not.
func baseTestRunPort(t *testing.T, nginxImage string, nginxIndexHTMLSnippet string, loopbackIsolationEnabled bool) {
	expectedIsolationErr := ""
	if loopbackIsolationEnabled {
		expectedIsolationErr = testutil.ExpectedConnectionRefusedError
	}

	hostIP, err := nettestutil.NonLoopbackIPv4()
	assert.NilError(t, err)
	type portTestCase struct {
		listenIP         net.IP
		connectIP        net.IP
		hostPort         string
		containerPort    string
		connectURLPort   int
		runShouldSuccess bool
		err              string
	}
	lo := net.ParseIP("127.0.0.1")
	zeroIP := net.ParseIP("0.0.0.0")
	testCases := []portTestCase{
		{
			listenIP:         lo,
			connectIP:        lo,
			hostPort:         "8080",
			containerPort:    "80",
			connectURLPort:   8080,
			runShouldSuccess: true,
		},
		{
			// for https://github.com/containerd/nerdctl/issues/88
			listenIP:         hostIP,
			connectIP:        hostIP,
			hostPort:         "8080",
			containerPort:    "80",
			connectURLPort:   8080,
			runShouldSuccess: true,
		},
		{
			listenIP:         hostIP,
			connectIP:        lo,
			hostPort:         "8080",
			containerPort:    "80",
			connectURLPort:   8080,
			err:              expectedIsolationErr,
			runShouldSuccess: true,
		},
		{
			listenIP:         lo,
			connectIP:        hostIP,
			hostPort:         "8080",
			containerPort:    "80",
			connectURLPort:   8080,
			err:              expectedIsolationErr,
			runShouldSuccess: true,
		},
		{
			listenIP:         zeroIP,
			connectIP:        lo,
			hostPort:         "8080",
			containerPort:    "80",
			connectURLPort:   8080,
			runShouldSuccess: true,
		},
		{
			listenIP:         zeroIP,
			connectIP:        hostIP,
			hostPort:         "8080",
			containerPort:    "80",
			connectURLPort:   8080,
			runShouldSuccess: true,
		},
		{
			listenIP:         lo,
			connectIP:        lo,
			hostPort:         "7000-7005",
			containerPort:    "79-84",
			connectURLPort:   7001,
			runShouldSuccess: true,
		},
		{
			listenIP:         hostIP,
			connectIP:        hostIP,
			hostPort:         "7000-7005",
			containerPort:    "79-84",
			connectURLPort:   7001,
			runShouldSuccess: true,
		},
		{
			listenIP:         hostIP,
			connectIP:        lo,
			hostPort:         "7000-7005",
			containerPort:    "79-84",
			connectURLPort:   7001,
			err:              expectedIsolationErr,
			runShouldSuccess: true,
		},
		{
			listenIP:         lo,
			connectIP:        hostIP,
			hostPort:         "7000-7005",
			containerPort:    "79-84",
			connectURLPort:   7001,
			err:              expectedIsolationErr,
			runShouldSuccess: true,
		},
		{
			listenIP:         zeroIP,
			connectIP:        hostIP,
			hostPort:         "7000-7005",
			containerPort:    "79-84",
			connectURLPort:   7001,
			runShouldSuccess: true,
		},
		{
			listenIP:         zeroIP,
			connectIP:        lo,
			hostPort:         "7000-7005",
			containerPort:    "80-85",
			connectURLPort:   7001,
			err:              "error after 5 attempts",
			runShouldSuccess: true,
		},
		{
			listenIP:         zeroIP,
			connectIP:        lo,
			hostPort:         "7000-7005",
			containerPort:    "80",
			connectURLPort:   7000,
			runShouldSuccess: true,
		},
		{
			listenIP:         zeroIP,
			connectIP:        lo,
			hostPort:         "7000-7005",
			containerPort:    "80",
			connectURLPort:   7005,
			err:              testutil.ExpectedConnectionRefusedError,
			runShouldSuccess: true,
		},
		{
			listenIP:         zeroIP,
			connectIP:        lo,
			hostPort:         "7000-7005",
			containerPort:    "79-85",
			connectURLPort:   7005,
			err:              "invalid ranges specified for container and host Ports",
			runShouldSuccess: false,
		},
	}

	testCase := nerdtest.Setup()
	testCase.NoParallel = true
	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		for i := range testCases {
			data.Labels().Set(fmt.Sprintf("container-%d", i), data.Identifier(fmt.Sprintf("container-%d", i)))
		}
	}

	for i, tc := range testCases {
		testCase.SubTests = append(testCase.SubTests, &test.Case{
			Description: fmt.Sprintf("%+v", tc),
			NoParallel:  true,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Labels().Get(fmt.Sprintf("container-%d", i)))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				testContainerName := data.Labels().Get(fmt.Sprintf("container-%d", i))
				pFlag := fmt.Sprintf("%s:%s:%s", tc.listenIP.String(), tc.hostPort, tc.containerPort)
				helpers.T().Log("pFlag=", pFlag, ", container=", testContainerName)
				return helpers.Command("run", "-d",
					"--name", testContainerName,
					"-p", pFlag,
					nginxImage)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				if tc.runShouldSuccess {
					return &test.Expected{
						ExitCode: expect.ExitCodeSuccess,
						Output: func(stdout string, t tig.T) {
							connectURL := fmt.Sprintf("http://%s:%d", tc.connectIP.String(), tc.connectURLPort)
							t.Log("connectURL=", connectURL)

							resp, err := nettestutil.HTTPGet(connectURL, 5, false)
							if tc.err != "" {
								assert.ErrorContains(t, err, tc.err)
								return
							}
							assert.NilError(t, err)
							respBody, err := io.ReadAll(resp.Body)
							assert.NilError(t, err)
							assert.Assert(t, strings.Contains(string(respBody), nginxIndexHTMLSnippet))
						},
					}
				}

				return &test.Expected{
					ExitCode: expect.ExitCodeGenericFail,
				}
			},
		})
	}

	testCase.Run(t)
}
