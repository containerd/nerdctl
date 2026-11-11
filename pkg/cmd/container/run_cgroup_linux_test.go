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

// fallbackDefaults lists the entries withDeviceCgroupRules is expected to append
// on top of specDeviceRules: those already covered by the spec are skipped, so
// only the mknod wildcards and /dev/net/tun are left. Written out literally so
// the test does not read its expectations back from the implementation.
func fallbackDefaults() []specs.LinuxDeviceCgroup {
	return []specs.LinuxDeviceCgroup{
		{Allow: true, Type: "c", Access: "m"},                                              // mknod for any char device
		{Allow: true, Type: "b", Access: "m"},                                              // mknod for any block device
		{Allow: true, Type: "c", Major: int64Ptr(10), Minor: int64Ptr(200), Access: "rwm"}, // /dev/net/tun
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
		specRules := specDeviceRules()
		rules := parse(t, "c 189:* rmw", "b 8:0 r", "a *:* rwm")
		assert.NilError(t, withDeviceCgroupRules(rules)(context.Background(), nil, nil, s))

		want := make([]specs.LinuxDeviceCgroup, 0, len(specRules)+len(fallbackDefaults())+len(rules))
		want = append(want, specRules...)
		want = append(want, fallbackDefaults()...)
		want = append(want, rules...)
		assert.DeepEqual(t, s.Linux.Resources.Devices, want)

		devices := s.Linux.Resources.Devices

		// /dev/net/tun is one of the grants that make the list self-contained
		// and the spec does not carry it, so it has to come from the defaults.
		// The expectation below is written out as a literal instead of being
		// read back from defaultDeviceCgroupRules, so dropping or changing the
		// grant fails this test.
		tun := specs.LinuxDeviceCgroup{
			Allow: true, Type: "c", Major: int64Ptr(10), Minor: int64Ptr(200), Access: "rwm",
		}
		got := indexOfDevice(devices, tun)
		assert.Assert(t, got >= 0, "missing /dev/net/tun grant")
		assert.Assert(t, got >= len(specRules),
			"/dev/net/tun grant must follow the rules already in the spec, got index %d", got)
		assert.Assert(t, got < len(devices)-len(rules),
			"/dev/net/tun grant must precede the user rules, got index %d of %d", got, len(devices))

		// The deny-all rule must stay ahead of every grant: the device cgroup
		// applies its rules in order, so a deny-all behind a grant cancels it.
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
		want = append(want, fallbackDefaults()...)
		want = append(want, rules...)
		assert.DeepEqual(t, s.Linux.Resources.Devices, want)

		// The user rule has to come after the deny-all rule, or it is dropped.
		last := s.Linux.Resources.Devices[len(s.Linux.Resources.Devices)-1]
		assert.Equal(t, true, last.Allow)
		assert.Equal(t, int64(189), *last.Major)
	})

	t.Run("keeps an explicit read-only device mapping", func(t *testing.T) {
		s := newSpec()
		// oci.WithDevices maps --device /dev/net/tun:r to exactly this rule.
		readOnlyTun := specs.LinuxDeviceCgroup{
			Allow: true, Type: "c", Major: int64Ptr(10), Minor: int64Ptr(200), Access: "r",
		}
		s.Linux.Resources.Devices = append(s.Linux.Resources.Devices, readOnlyTun)

		rules := parse(t, "c 189:* rmw")
		assert.NilError(t, withDeviceCgroupRules(rules)(context.Background(), nil, nil, s))

		devices := s.Linux.Resources.Devices

		// The fallback grants must not widen the mapping to "rwm": the only rule
		// for 10:200 has to be the read-only one that was already in the spec.
		wideTun := specs.LinuxDeviceCgroup{
			Allow: true, Type: "c", Major: int64Ptr(10), Minor: int64Ptr(200), Access: "rwm",
		}
		assert.Equal(t, -1, indexOfDevice(devices, wideTun))
		assert.Equal(t, len(specDeviceRules()), indexOfDevice(devices, readOnlyTun))

		specRules := specDeviceRules()
		want := make([]specs.LinuxDeviceCgroup, 0, len(specRules)+1+len(fallbackDefaults())+len(rules))
		want = append(want, specRules...)
		want = append(want, readOnlyTun)
		// /dev/net/tun is covered by the mapping, so only the mknod wildcards and
		// the user rules follow.
		want = append(want, fallbackDefaults()[:2]...)
		want = append(want, rules...)
		assert.DeepEqual(t, devices, want)
	})

	t.Run("leaves wildcard major and minor unset", func(t *testing.T) {
		s := newSpec()
		rules := parse(t, "c *:* rwm", "c 189:* rmw")
		assert.NilError(t, withDeviceCgroupRules(rules)(context.Background(), nil, nil, s))

		// crun writes a present major/minor verbatim, so -1 would reach the kernel
		// on cgroup v1; an unset field is what both runtimes read as "any".
		devices := s.Linux.Resources.Devices
		wildcard := devices[len(devices)-2]
		assert.Equal(t, "c", wildcard.Type)
		assert.Assert(t, wildcard.Major == nil && wildcard.Minor == nil,
			"a fully wildcard rule must not spell out major or minor")

		partial := devices[len(devices)-1]
		assert.Equal(t, int64(189), *partial.Major)
		assert.Assert(t, partial.Minor == nil, "a wildcard minor must stay unset")
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

	// /dev/net/tun: fixed major/minor, and the spec does not carry it.
	assert.Assert(t, containsDefault(specs.LinuxDeviceCgroup{
		Allow: true, Type: "c", Major: int64Ptr(10), Minor: int64Ptr(200), Access: "rwm",
	}), "missing /dev/net/tun grant")
}

func containsDefault(want specs.LinuxDeviceCgroup) bool {
	return indexOfDevice(defaultDeviceCgroupRules, want) >= 0
}

// indexOfDevice returns the position of want in devices, or -1.
func indexOfDevice(devices []specs.LinuxDeviceCgroup, want specs.LinuxDeviceCgroup) int {
	for i, d := range devices {
		if sameDevice(d, want) {
			return i
		}
	}
	return -1
}

func sameDevice(a, b specs.LinuxDeviceCgroup) bool {
	return a.Allow == b.Allow && a.Type == b.Type && a.Access == b.Access &&
		sameInt64(a.Major, b.Major) && sameInt64(a.Minor, b.Minor)
}

func sameInt64(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
