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

package netutil

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestParseIPAMRange(t *testing.T) {
	t.Parallel()
	type testCase struct {
		subnet   string
		gateway  string
		iprange  string
		expected *IPAMRange
		err      string
	}
	testCases := []testCase{
		{
			subnet:   "",
			expected: nil,
			err:      "failed to parse subnet",
		},
		{
			subnet:   "10.1.100.1/24",
			expected: nil,
			err:      "unexpected subnet",
		},
		{
			subnet: "10.1.100.0/24",
			expected: &IPAMRange{
				Subnet:  "10.1.100.0/24",
				Gateway: "10.1.100.1",
			},
		},
		{
			subnet:  "10.1.100.0/24",
			gateway: "10.1.10.100",
			err:     "no matching subnet",
		},
		{
			subnet:  "10.1.100.0/24",
			gateway: "10.1.100.100",
			expected: &IPAMRange{
				Subnet:  "10.1.100.0/24",
				Gateway: "10.1.100.100",
			},
		},
		{
			subnet:  "10.1.100.0/23",
			gateway: "10.1.102.1",
			err:     "no matching subnet",
		},
		{
			subnet:  "10.1.0.0/16",
			iprange: "10.10.10.0/24",
			err:     "no matching subnet",
		},
		{
			subnet:  "10.1.0.0/16",
			iprange: "10.1.100.0/24",
			expected: &IPAMRange{
				Subnet:     "10.1.0.0/16",
				Gateway:    "10.1.0.1",
				IPRange:    "10.1.100.0/24",
				RangeStart: "10.1.100.1",
				RangeEnd:   "10.1.100.255",
			},
		},
		{
			subnet:  "10.1.100.0/23",
			iprange: "10.1.100.0/25",
			expected: &IPAMRange{
				Subnet:     "10.1.100.0/23",
				Gateway:    "10.1.100.1",
				IPRange:    "10.1.100.0/25",
				RangeStart: "10.1.100.1",
				RangeEnd:   "10.1.100.127",
			},
		},
	}
	for _, tc := range testCases {
		got, err := parseIPAMRange(tc.subnet, tc.gateway, tc.iprange)
		if tc.err != "" {
			assert.ErrorContains(t, err, tc.err)
		} else {
			assert.NilError(t, err)
			assert.Equal(t, *tc.expected, *got)
		}
	}
}
