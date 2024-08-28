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

package hostsstore

import (
	"net"
	"strings"
	"testing"

	types100 "github.com/containernetworking/cni/pkg/types/100"
	"gotest.tools/v3/assert"
)

func TestCreateLine(t *testing.T) {
	type testCase struct {
		thatIP       string
		thatNetwork  string
		thatHostname string // nerdctl run --hostname
		thatName     string // nerdctl run --name
		myNetwork    string
		expected     string
	}
	testCases := []testCase{
		{
			thatIP:       "10.4.2.2",
			thatNetwork:  "n1",
			thatHostname: "bar",
			thatName:     "foo",
			myNetwork:    "n1",
			expected:     "bar bar.n1 foo foo.n1",
		},
		{
			thatIP:       "10.4.2.3",
			thatNetwork:  "n1",
			thatHostname: "bar",
			myNetwork:    "n1",
			expected:     "bar bar.n1",
		},
		{
			thatIP:       "10.4.2.4",
			thatNetwork:  "bridge",
			thatHostname: "bar",
			myNetwork:    "n1",
			expected:     "",
		},
		{
			thatIP:      "10.4.2.5",
			thatNetwork: "n1",
			thatName:    "foo",
			myNetwork:   "bridge",
			expected:    "",
		},
		{
			thatIP:      "10.4.2.6",
			thatNetwork: "n1",
			thatName:    "foo",
			myNetwork:   "n2",
			expected:    "",
		},
	}
	for _, tc := range testCases {
		thatMeta := &Meta{
			ID: "984d63ce45ae",
			Networks: map[string]*types100.Result{
				tc.thatNetwork: {
					Interfaces: []*types100.Interface{
						{
							Name: "eth0",
						},
					},
					IPs: []*types100.IPConfig{
						{
							Address: net.IPNet{IP: net.ParseIP(tc.thatIP)},
						},
					},
				},
			},
			Hostname: tc.thatHostname,
			Name:     tc.thatName,
		}

		myNetworks := map[string]struct{}{
			tc.myNetwork: {},
		}
		lines := createLine(tc.thatNetwork, thatMeta, myNetworks)
		line := strings.Join(lines, " ")
		t.Logf("tc=%+v, line=%q", tc, line)
		assert.Equal(t, tc.expected, line)
	}
}
