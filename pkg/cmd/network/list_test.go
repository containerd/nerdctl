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

package network

import (
	"testing"

	"github.com/containernetworking/cni/libcni"
	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/netutil"
)

func TestNetworkMatchesFilter(t *testing.T) {
	t.Parallel()
	labels := map[string]string{"env": "prod", "tier": "web"}
	net := &netutil.NetworkConfig{
		NetworkConfigList: &libcni.NetworkConfigList{Name: "frontend"},
		NerdctlLabels:     &labels,
	}

	testCases := []struct {
		name     string
		filters  []string
		expected bool
	}{
		{"no filters", nil, true},
		{"matching name", []string{"name=frontend"}, true},
		{"one of multiple names", []string{"name=backend", "name=frontend"}, true},
		{"all labels", []string{"label=env=prod", "label=tier=web"}, true},
		{"one of multiple labels", []string{"label=env=dev", "label=tier=web"}, false},
		{"matching name and label", []string{"name=frontend", "label=env=prod"}, true},
		{"matching name only", []string{"name=frontend", "label=env=dev"}, false},
		{"matching label only", []string{"name=backend", "label=env=prod"}, false},
		{"no match", []string{"name=backend", "label=env=dev"}, false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			labelFilters, nameFilters, err := getNetworkFilterFuncs(tc.filters)
			assert.NilError(t, err)
			assert.Equal(t, networkMatchesFilter(net, labelFilters, nameFilters), tc.expected)
		})
	}
}
