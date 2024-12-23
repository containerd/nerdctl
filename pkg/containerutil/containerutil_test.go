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

package containerutil

import (
	"reflect"
	"testing"
)

func TestParseExtraHosts(t *testing.T) {
	tests := []struct {
		name           string
		extraHosts     []string
		hostGateway    string
		separator      string
		expected       []string
		expectedErrStr string
	}{
		{
			name:     "NoExtraHosts",
			expected: []string{},
		},
		{
			name:       "ExtraHosts",
			extraHosts: []string{"localhost:127.0.0.1", "localhost:[::1]"},
			separator:  ":",
			expected:   []string{"localhost:127.0.0.1", "localhost:[::1]"},
		},
		{
			name:       "EqualsSeperator",
			extraHosts: []string{"localhost:127.0.0.1", "localhost:[::1]"},
			separator:  "=",
			expected:   []string{"localhost=127.0.0.1", "localhost=[::1]"},
		},
		{
			name:           "InvalidExtraHostFormat",
			extraHosts:     []string{"localhost"},
			expectedErrStr: "bad format for add-host: \"localhost\"",
		},
		{
			name:           "ErrorOnHostGatewayExtraHostWithNoHostGatewayIPSet",
			extraHosts:     []string{"localhost:host-gateway"},
			separator:      ":",
			expectedErrStr: "unable to derive the IP value for host-gateway",
		},
		{
			name:        "HostGatewayIP",
			extraHosts:  []string{"localhost:host-gateway"},
			hostGateway: "10.10.0.1",
			separator:   ":",
			expected:    []string{"localhost:10.10.0.1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			extraHosts, err := ParseExtraHosts(test.extraHosts, test.hostGateway, test.separator)
			if err != nil && err.Error() != test.expectedErrStr {
				t.Fatalf("expected '%s', actual '%v'", test.expectedErrStr, err)
			} else if err == nil && test.expectedErrStr != "" {
				t.Fatalf("expected error '%s' but got none", test.expectedErrStr)
			}

			if !reflect.DeepEqual(test.expected, extraHosts) {
				t.Fatalf("expected %v, actual %v", test.expected, extraHosts)
			}
		})
	}
}
