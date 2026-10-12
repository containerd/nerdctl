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
	"context"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"
	"gotest.tools/v3/assert"

	"github.com/containerd/containerd/v2/pkg/oci"
)

func TestWithDeviceCgroupRules(t *testing.T) {
	t.Parallel()

	// The spec containerd hands over already denies every device, and allow rules
	// take effect only when they come after that entry.
	newSpec := func() *oci.Spec {
		return &oci.Spec{
			Linux: &specs.Linux{
				Resources: &specs.LinuxResources{
					Devices: []specs.LinuxDeviceCgroup{
						{Allow: false, Access: "rwm"},
					},
				},
			},
		}
	}

	parse := func(t *testing.T, rules ...string) []specs.LinuxDeviceCgroup {
		t.Helper()
		parsed := make([]specs.LinuxDeviceCgroup, 0, len(rules))
		for _, rule := range rules {
			p, err := ParseDeviceCgroupRule(rule)
			assert.NilError(t, err)
			parsed = append(parsed, p)
		}
		return parsed
	}

	t.Run("appends the rules after the deny-all entry, in the given order", func(t *testing.T) {
		s := newSpec()
		opt := withDeviceCgroupRules(parse(t, "c 189:* rmw", "b 8:0 r", "a *:* rwm"))
		assert.NilError(t, opt(context.Background(), nil, nil, s))

		devices := s.Linux.Resources.Devices
		assert.Equal(t, 4, len(devices))

		assert.Equal(t, false, devices[0].Allow)

		assert.Equal(t, true, devices[1].Allow)
		assert.Equal(t, "c", devices[1].Type)
		assert.Equal(t, int64(189), *devices[1].Major)
		assert.Equal(t, int64(-1), *devices[1].Minor)
		assert.Equal(t, "rmw", devices[1].Access)

		assert.Equal(t, "b", devices[2].Type)
		assert.Equal(t, int64(8), *devices[2].Major)
		assert.Equal(t, int64(0), *devices[2].Minor)
		assert.Equal(t, "r", devices[2].Access)

		assert.Equal(t, "a", devices[3].Type)
		assert.Equal(t, int64(-1), *devices[3].Major)
		assert.Equal(t, int64(-1), *devices[3].Minor)
	})

	t.Run("leaves the device list untouched when no rule is given", func(t *testing.T) {
		s := newSpec()
		opt := withDeviceCgroupRules(nil)
		assert.NilError(t, opt(context.Background(), nil, nil, s))

		devices := s.Linux.Resources.Devices
		assert.Equal(t, 1, len(devices))
		assert.Equal(t, false, devices[0].Allow)
	})
}
