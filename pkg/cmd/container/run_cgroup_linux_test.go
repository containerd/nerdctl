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

// specDeviceRules mirrors what the spec already holds when withDeviceCgroupRules
// runs: oci.WithDefaultSpec starts the list with a deny-all rule, and
// oci.WithDefaultUnixDevices appends the standard device grants behind it.
func specDeviceRules() []specs.LinuxDeviceCgroup {
	return []specs.LinuxDeviceCgroup{
		{Allow: false, Access: "rwm"},
		{Allow: true, Type: "c", Major: int64Ptr(1), Minor: int64Ptr(3), Access: "rwm"}, // /dev/null
		{Allow: true, Type: "c", Major: int64Ptr(1), Minor: int64Ptr(8), Access: "rwm"}, // /dev/random
		{Allow: true, Type: "c", Major: int64Ptr(1), Minor: int64Ptr(7), Access: "rwm"}, // /dev/full
		{Allow: true, Type: "c", Major: int64Ptr(5), Minor: int64Ptr(0), Access: "rwm"}, // /dev/tty
		{Allow: true, Type: "c", Major: int64Ptr(1), Minor: int64Ptr(5), Access: "rwm"}, // /dev/zero
		{Allow: true, Type: "c", Major: int64Ptr(1), Minor: int64Ptr(9), Access: "rwm"}, // /dev/urandom
		{Allow: true, Type: "c", Major: int64Ptr(5), Minor: int64Ptr(1), Access: "rwm"}, // /dev/console
		{Allow: true, Type: "c", Major: int64Ptr(136), Access: "rwm"},                   // /dev/pts
		{Allow: true, Type: "c", Major: int64Ptr(5), Minor: int64Ptr(2), Access: "rwm"}, // /dev/ptmx
	}
}

func TestWithDeviceCgroupRules(t *testing.T) {
	t.Parallel()

	newSpec := func() *oci.Spec {
		return &oci.Spec{
			Linux: &specs.Linux{
				Resources: &specs.LinuxResources{Devices: specDeviceRules()},
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

	t.Run("appends the defaults and the rules behind the existing spec rules", func(t *testing.T) {
		s := newSpec()
		rules := parse(t, "c 189:* rmw", "b 8:0 r", "a *:* rwm")
		assert.NilError(t, withDeviceCgroupRules(rules)(context.Background(), nil, nil, s))

		want := append(specDeviceRules(), defaultDeviceCgroupRules...)
		want = append(want, rules...)
		assert.DeepEqual(t, s.Linux.Resources.Devices, want)

		// The deny-all rule must stay ahead of every grant: the device cgroup
		// applies its rules in order, so a deny-all behind a grant cancels it.
		devices := s.Linux.Resources.Devices
		assert.Equal(t, false, devices[0].Allow)
		assert.Equal(t, "", devices[0].Type)
	})

	t.Run("keeps the --device grants ahead of the appended rules", func(t *testing.T) {
		s := newSpec()
		deviceGrant := specs.LinuxDeviceCgroup{
			Allow: true, Type: "b", Major: int64Ptr(8), Minor: int64Ptr(0), Access: "rwm",
		}
		s.Linux.Resources.Devices = append(s.Linux.Resources.Devices, deviceGrant)

		rules := parse(t, "c 189:* rmw")
		assert.NilError(t, withDeviceCgroupRules(rules)(context.Background(), nil, nil, s))

		want := append(specDeviceRules(), deviceGrant)
		want = append(want, defaultDeviceCgroupRules...)
		want = append(want, rules...)
		assert.DeepEqual(t, s.Linux.Resources.Devices, want)

		// The user rule has to come after the deny-all rule, or it is dropped.
		last := s.Linux.Resources.Devices[len(s.Linux.Resources.Devices)-1]
		assert.Equal(t, true, last.Allow)
		assert.Equal(t, int64(189), *last.Major)
	})

	t.Run("leaves the device list untouched when no rule is given", func(t *testing.T) {
		s := newSpec()
		assert.NilError(t, withDeviceCgroupRules(nil)(context.Background(), nil, nil, s))

		assert.DeepEqual(t, s.Linux.Resources.Devices, specDeviceRules())
	})
}

// The defaults must not carry an explicit -1 wildcard: crun writes a present
// major/minor verbatim, so "c -1:-1 m" would be rejected by the kernel on
// cgroup v1. Omitted fields are what both runc and crun read as a wildcard.
func TestDefaultDeviceCgroupRulesWildcards(t *testing.T) {
	t.Parallel()

	for _, d := range defaultDeviceCgroupRules {
		if d.Major != nil {
			assert.Assert(t, *d.Major != -1, "unexpected explicit -1 major in %+v", d)
		}
		if d.Minor != nil {
			assert.Assert(t, *d.Minor != -1, "unexpected explicit -1 minor in %+v", d)
		}
	}

	// mknod for any device: both wildcards omitted.
	assert.Assert(t, containsDefault(specs.LinuxDeviceCgroup{Allow: true, Type: "c", Access: "m"}),
		"missing mknod char wildcard")
	assert.Assert(t, containsDefault(specs.LinuxDeviceCgroup{Allow: true, Type: "b", Access: "m"}),
		"missing mknod block wildcard")

	// /dev/pts: wildcard minor omitted.
	assert.Assert(t, containsDefault(specs.LinuxDeviceCgroup{
		Allow: true, Type: "c", Major: int64Ptr(136), Access: "rwm",
	}), "missing /dev/pts wildcard minor")
}

func containsDefault(want specs.LinuxDeviceCgroup) bool {
	for _, d := range defaultDeviceCgroupRules {
		if d.Allow == want.Allow && d.Type == want.Type && d.Access == want.Access &&
			sameInt64(d.Major, want.Major) && sameInt64(d.Minor, want.Minor) {
			return true
		}
	}
	return false
}

func sameInt64(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
