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

	"github.com/containerd/nerdctl/v2/pkg/labels"
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

func TestGetContainerVolumes_Indexed(t *testing.T) {
	m0 := `{"Type":"volume","Name":"vol-0","Source":"/var/lib/vol-0","Destination":"/mnt/vol-0"}`
	m1 := `{"Type":"volume","Name":"vol-1","Source":"/var/lib/vol-1","Destination":"/mnt/vol-1"}`
	m2 := `{"Type":"volume","Name":"vol-2","Source":"/var/lib/vol-2","Destination":"/mnt/vol-2"}`

	rawJSON := "[" + m0 + "," + m1 + "," + m2 + "]"

	indexedLabels := map[string]string{
		"nerdctl/mounts.0": m0,
		"nerdctl/mounts.1": m1,
		"nerdctl/mounts.2": m2,
	}

	legacyLabels := map[string]string{
		labels.Mounts: rawJSON,
	}

	indexedResult := GetContainerVolumes(indexedLabels)
	legacyResult := GetContainerVolumes(legacyLabels)

	if len(indexedResult) == 0 {
		t.Fatal("Expected to extract volumes from indexed labels, but got 0 results")
	}

	if len(indexedResult) != len(legacyResult) {
		t.Errorf("Mismatched output! Indexed found %d volumes, Legacy found %d volumes.",
			len(indexedResult), len(legacyResult))
	}

	if indexedResult[0].Name != "vol-0" {
		t.Errorf("Expected first volume to be named 'vol-0', got '%s'", indexedResult[0].Name)
	}
	if len(indexedResult) > 2 && indexedResult[2].Name != "vol-2" {
		t.Errorf("Expected third volume to be named 'vol-2', got '%s'", indexedResult[2].Name)
	}
}
