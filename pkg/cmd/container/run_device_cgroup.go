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

	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/oci"
)

// defaultDeviceCgroupRules holds the entries that runc and crun add to every
// container's device allow list (runc: specconv.AllowedDevices; crun keeps the
// same list for parity with runc).
//
// containerd's oci.WithDefaultUnixDevices already lists most of these devices
// explicitly (/dev/null, /dev/random, /dev/full, /dev/tty, /dev/zero,
// /dev/urandom, /dev/console, /dev/pts, /dev/ptmx), but the mknod wildcards and
// /dev/net/tun are in no spec, and a runtime that applies the spec's device list
// verbatim would deny them. Listing the whole set keeps the allow list
// self-contained; on runc and crun the duplicate grants are no-ops.
//
// Wildcards are deliberately left as nil pointers: runc reads a missing
// major/minor as -1, whereas crun writes a present -1 literally ("c -1:-1 m"),
// which the kernel rejects on cgroup v1.
//
// Keep in sync with runc's specconv.AllowedDevices (unchanged from v1.1.15
// through v1.5.2, the current stable release).
var defaultDeviceCgroupRules = []specs.LinuxDeviceCgroup{
	// allow mknod for any device
	{Allow: true, Type: "c", Access: "m"},
	{Allow: true, Type: "b", Access: "m"},
	{Allow: true, Type: "c", Major: int64Ptr(1), Minor: int64Ptr(3), Access: "rwm"},    // /dev/null
	{Allow: true, Type: "c", Major: int64Ptr(1), Minor: int64Ptr(8), Access: "rwm"},    // /dev/random
	{Allow: true, Type: "c", Major: int64Ptr(1), Minor: int64Ptr(7), Access: "rwm"},    // /dev/full
	{Allow: true, Type: "c", Major: int64Ptr(5), Minor: int64Ptr(0), Access: "rwm"},    // /dev/tty
	{Allow: true, Type: "c", Major: int64Ptr(1), Minor: int64Ptr(5), Access: "rwm"},    // /dev/zero
	{Allow: true, Type: "c", Major: int64Ptr(1), Minor: int64Ptr(9), Access: "rwm"},    // /dev/urandom
	{Allow: true, Type: "c", Major: int64Ptr(136), Access: "rwm"},                      // /dev/pts
	{Allow: true, Type: "c", Major: int64Ptr(5), Minor: int64Ptr(2), Access: "rwm"},    // /dev/ptmx
	{Allow: true, Type: "c", Major: int64Ptr(10), Minor: int64Ptr(200), Access: "rwm"}, // /dev/net/tun
}

func int64Ptr(v int64) *int64 {
	return &v
}

// withDeviceCgroupRules adds the given rules to the device allow list.
//
// The rules already in the spec stay in front: oci.WithDefaultSpec starts the
// list with a deny-all rule and oci.WithDefaultUnixDevices appends the standard
// device grants, and a device cgroup applies its rules in order, so a deny-all
// rule placed behind a grant cancels it. The defaults and the user rules are
// therefore appended last, and nothing the runtime adds implicitly is needed to
// make the list self-contained.
//
// A default entry is skipped when the spec already grants that device, as
// oci.WithDevices does for --device: mapping /dev/net/tun as read-only must not
// be widened to "rwm" by the fallback grants. CDI devices can carry the same
// narrower permissions, and their rules are injected by oci.WithCDIDevices, so
// this option has to be applied after CDI injection (see create.go) for the
// coverage check to see them.
func withDeviceCgroupRules(rules []specs.LinuxDeviceCgroup) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		if len(rules) == 0 {
			return nil
		}
		existing := s.Linux.Resources.Devices
		devices := make([]specs.LinuxDeviceCgroup, 0, len(existing)+len(defaultDeviceCgroupRules)+len(rules))
		devices = append(devices, existing...)
		for _, d := range defaultDeviceCgroupRules {
			if deviceCovered(existing, d) {
				continue
			}
			devices = append(devices, d)
		}
		devices = append(devices, rules...)
		s.Linux.Resources.Devices = devices
		return nil
	}
}

// deviceCovered reports whether devices already allows the device that rule
// grants, in which case appending rule would be redundant. Only allow entries
// count: the deny-all entry from oci.WithDefaultSpec covers nothing.
func deviceCovered(devices []specs.LinuxDeviceCgroup, rule specs.LinuxDeviceCgroup) bool {
	for _, d := range devices {
		if !d.Allow {
			continue
		}
		if d.Type != rule.Type && d.Type != "a" {
			continue
		}
		if coversNumber(d.Major, rule.Major) && coversNumber(d.Minor, rule.Minor) {
			return true
		}
	}
	return false
}

// coversNumber reports whether the number already in the allow list covers the
// one a default rule grants. A wildcard covers anything, but a wildcard default
// (the mknod entries) is only covered by another wildcard: a grant for one
// device does not allow creating device nodes.
func coversNumber(existing, want *int64) bool {
	if want == nil {
		return existing == nil
	}
	return existing == nil || *existing == *want
}
