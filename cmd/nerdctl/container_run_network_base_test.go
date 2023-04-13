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

package main

import (
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/containerd/nerdctl/pkg/testutil/nettestutil"
	"gotest.tools/v3/assert"
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
	type testCase struct {
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
	testCases := []testCase{
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
			err:              "error after 30 attempts",
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

	tID := testutil.Identifier(t)
	for i, tc := range testCases {
		i := i
		tc := tc
		tcName := fmt.Sprintf("%+v", tc)
		t.Run(tcName, func(t *testing.T) {
			testContainerName := fmt.Sprintf("%s-%d", tID, i)
			base := testutil.NewBase(t)
			defer base.Cmd("rm", "-f", testContainerName).Run()
			pFlag := fmt.Sprintf("%s:%s:%s", tc.listenIP.String(), tc.hostPort, tc.containerPort)
			connectURL := fmt.Sprintf("http://%s:%d", tc.connectIP.String(), tc.connectURLPort)
			t.Logf("pFlag=%q, connectURL=%q", pFlag, connectURL)
			cmd := base.Cmd("run", "-d",
				"--name", testContainerName,
				"-p", pFlag,
				nginxImage)
			if tc.runShouldSuccess {
				cmd.AssertOK()
			} else {
				cmd.AssertFail()
				return
			}

			resp, err := nettestutil.HTTPGet(connectURL, 30, false)
			if tc.err != "" {
				assert.ErrorContains(t, err, tc.err)
				return
			}
			assert.NilError(t, err)
			respBody, err := io.ReadAll(resp.Body)
			assert.NilError(t, err)
			assert.Assert(t, strings.Contains(string(respBody), nginxIndexHTMLSnippet))
		})
	}

}

func valuesOfMapStringString(m map[string]string) map[string]struct{} {
	res := make(map[string]struct{})
	for _, v := range m {
		res[v] = struct{}{}
	}
	return res
}

func extractHostPort(portMapping string) (string, error) {
	// Regular expression to extract host port from port mapping information
	re := regexp.MustCompile(`\d+/tcp ->.*?0.0.0.0:(?P<portNumber>\d{1,5}).*?`)

	// Find the matches
	matches := re.FindStringSubmatch(portMapping)

	// Check if there is a match
	if len(matches) < 2 {
		return "", fmt.Errorf("could not extract host port from port mapping: %s", portMapping)
	}

	// Extract the host port number
	hostPort := matches[1]

	return hostPort, nil
}
