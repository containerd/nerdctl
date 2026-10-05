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
	config, err := libcni.ConfListFromBytes([]byte(`{"cniVersion":"1.0.0","name":"frontend","plugins":[{"type":"bridge"}]}`))
	assert.NilError(t, err)
	net := &netutil.NetworkConfig{
		NetworkConfigList: config,
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
		{"matching driver", []string{"driver=bridge"}, true},
		{"nonmatching driver", []string{"driver=macvlan"}, false},
		{"one of multiple drivers", []string{"driver=macvlan", "driver=bridge"}, true},
		{"matching driver and label", []string{"driver=bridge", "label=env=prod"}, true},
		{"matching driver only", []string{"driver=bridge", "label=env=dev"}, false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			labelFilters, nameFilters, driverFilters, err := getNetworkFilterFuncs(tc.filters)
			assert.NilError(t, err)
			assert.Equal(t, networkMatchesFilter(net, labelFilters, nameFilters, driverFilters), tc.expected)
		})
	}
}

func TestNetworkFilterRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		filter string
		want   string
	}{
		{"name", "bad format of filter"},
		{"label", "bad format of filter"},
		{"names=frontend", "invalid filter 'names'"},
		{"labels=env=prod", "invalid filter 'labels'"},
	} {
		t.Run(tc.filter, func(t *testing.T) {
			_, _, _, err := getNetworkFilterFuncs([]string{tc.filter})
			assert.ErrorContains(t, err, tc.want)
		})
	}
}
