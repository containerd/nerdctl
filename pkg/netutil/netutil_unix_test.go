//go:build unix

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
	"net"
	"testing"

	"github.com/Masterminds/semver/v3"
	"gotest.tools/v3/assert"
)

func TestGuessFirewallPluginVersion(t *testing.T) {

	type testCase struct {
		stderr   string
		expected string
		err      string
	}
	testCases := []testCase{
		{
			stderr:   "CNI firewall plugin v1.1.0\n",
			expected: "1.1.0",
		},
		{
			stderr:   "CNI firewall plugin v0.8.0\n",
			expected: "0.8.0",
		},
		{
			stderr:   "Foo\nCNI firewall plugin v123.456.789+beta.10\nBar\n",
			expected: "123.456.789+beta.10",
		},
		{
			stderr: "CNI firewall plugin version unknown\n",
			err:    semver.ErrInvalidSemVer.Error(),
		},
		{
			stderr: "",
			err:    "does not have any line that starts with \"CNI firewall plugin \"",
		},
		{
			stderr: "Foo\nBar\nBaz\n",
			err:    "does not have any line that starts with \"CNI firewall plugin \"",
		},
	}

	for _, tc := range testCases {
		got, err := guessFirewallPluginVersion(tc.stderr)
		if tc.err == "" {
			assert.NilError(t, err)
			assert.Equal(t, tc.expected, got.String())
		} else {
			assert.ErrorContains(t, err, tc.err)
		}
	}
}

// TestPairIPAMRangesIPRange covers matching repeatable --ip-range values to the
// subnet that contains each, and the errors for an unmatched or malformed range.
func TestPairIPAMRangesIPRange(t *testing.T) {
	t.Parallel()
	// parse turns the CIDR strings into the already-resolved subnets that
	// pairIPAMRanges takes, keeping each subtest readable.
	parse := func(t *testing.T, cidrs ...string) []*net.IPNet {
		t.Helper()
		subnets := make([]*net.IPNet, len(cidrs))
		for i, c := range cidrs {
			_, n, err := net.ParseCIDR(c)
			assert.NilError(t, err)
			subnets[i] = n
		}
		return subnets
	}

	t.Run("each ip-range pairs with its subnet regardless of order", func(t *testing.T) {
		subnets := parse(t, "10.6.0.0/16", "2001:db8:6::/64")
		// Given v6-first to prove the pairing is by containment, not by index.
		ipRanges := []string{"2001:db8:6::/80", "10.6.1.0/24"}
		ranges, findIPv4, _, err := pairIPAMRanges(subnets, nil, ipRanges, nil, true)
		assert.NilError(t, err)
		assert.Equal(t, true, findIPv4)
		got := map[string]string{}
		for _, r := range ranges {
			got[r[0].Subnet] = r[0].IPRange
		}
		assert.Equal(t, "10.6.1.0/24", got["10.6.0.0/16"])
		assert.Equal(t, "2001:db8:6::/80", got["2001:db8:6::/64"])
	})

	t.Run("an ip-range matching no subnet errors", func(t *testing.T) {
		_, _, _, err := pairIPAMRanges(parse(t, "10.6.0.0/16"), nil, []string{"192.168.1.0/24"}, nil, false)
		assert.ErrorContains(t, err, `no matching subnet for ip-range "192.168.1.0/24"`)
	})

	t.Run("an IPv4 ip-range with only an IPv6 subnet errors", func(t *testing.T) {
		_, _, _, err := pairIPAMRanges(parse(t, "2001:db8:6::/64"), nil, []string{"10.6.1.0/24"}, nil, true)
		assert.ErrorContains(t, err, `no matching subnet for ip-range "10.6.1.0/24"`)
	})

	t.Run("a second ip-range claiming the same subnet errors", func(t *testing.T) {
		_, _, _, err := pairIPAMRanges(parse(t, "10.6.0.0/16"), nil, []string{"10.6.1.0/24", "10.6.2.0/24"}, nil, false)
		assert.ErrorContains(t, err, `no matching subnet for ip-range "10.6.2.0/24"`)
	})

	t.Run("a malformed ip-range errors", func(t *testing.T) {
		_, _, _, err := pairIPAMRanges(parse(t, "10.6.0.0/16"), nil, []string{"bogus"}, nil, false)
		assert.ErrorContains(t, err, `failed to parse ip-range "bogus"`)
	})
}

// TestPairIPAMRangesAuxAddress exercises the aux-address side of pairIPAMRanges:
// each reserved address is matched to the subnet that contains it, recorded for
// inspect, and carved out of the range, while the network/gateway address and an
// address matching no subnet are rejected.
func TestPairIPAMRangesAuxAddress(t *testing.T) {
	t.Parallel()
	parse := func(t *testing.T, cidrs ...string) []*net.IPNet {
		t.Helper()
		subnets := make([]*net.IPNet, len(cidrs))
		for i, c := range cidrs {
			_, n, err := net.ParseCIDR(c)
			assert.NilError(t, err)
			subnets[i] = n
		}
		return subnets
	}

	t.Run("a reserved address is recorded and carved out of the range", func(t *testing.T) {
		ranges, _, aux, err := pairIPAMRanges(parse(t, "10.7.0.0/24"), nil, nil, map[string]string{"host": "10.7.0.5"}, false)
		assert.NilError(t, err)
		assert.Equal(t, 1, len(ranges))
		// The reservation is returned keyed by subnet for the caller to store, and
		// the range is split so .5 falls in the gap between the two sub-ranges.
		assert.DeepEqual(t, map[string]string{"host": "10.7.0.5"}, aux["10.7.0.0/24"])
		assert.Equal(t, 2, len(ranges[0]))
		assert.Equal(t, "10.7.0.4", ranges[0][0].RangeEnd)
		assert.Equal(t, "10.7.0.6", ranges[0][1].RangeStart)
	})

	t.Run("a reserved network address is rejected", func(t *testing.T) {
		_, _, _, err := pairIPAMRanges(parse(t, "10.7.0.0/24"), nil, nil, map[string]string{"net": "10.7.0.0"}, false)
		assert.ErrorContains(t, err, "Address already in use")
	})

	t.Run("a reserved gateway address is rejected", func(t *testing.T) {
		// With no explicit gateway the first address (.1) is the gateway, so an
		// aux-address on it collides.
		_, _, _, err := pairIPAMRanges(parse(t, "10.7.0.0/24"), nil, nil, map[string]string{"gw": "10.7.0.1"}, false)
		assert.ErrorContains(t, err, "Address already in use")
	})

	t.Run("an aux-address matching no subnet errors", func(t *testing.T) {
		_, _, _, err := pairIPAMRanges(parse(t, "10.7.0.0/24"), nil, nil, map[string]string{"x": "192.168.5.5"}, false)
		assert.ErrorContains(t, err, "no matching subnet for aux-address 192.168.5.5")
	})

	t.Run("dual-stack keeps each aux-address on its own family's subnet", func(t *testing.T) {
		ranges, _, aux, err := pairIPAMRanges(parse(t, "10.7.0.0/24", "fd00:7::/64"), nil, nil, map[string]string{"v4": "10.7.0.9", "v6": "fd00:7::9"}, true)
		assert.NilError(t, err)
		assert.Equal(t, 2, len(ranges))
		assert.DeepEqual(t, map[string]string{"v4": "10.7.0.9"}, aux["10.7.0.0/24"])
		assert.DeepEqual(t, map[string]string{"v6": "fd00:7::9"}, aux["fd00:7::/64"])
	})

	t.Run("multiple aux-addresses in one subnet split it into three ranges", func(t *testing.T) {
		ranges, _, aux, err := pairIPAMRanges(parse(t, "10.7.0.0/24"), nil, nil, map[string]string{"a": "10.7.0.5", "b": "10.7.0.9"}, false)
		assert.NilError(t, err)
		assert.Equal(t, 1, len(ranges))
		// Both reservations come back under the subnet key, and the subnet is
		// carved into three ranges with .5 and .9 sitting in the two gaps.
		assert.DeepEqual(t, map[string]string{"a": "10.7.0.5", "b": "10.7.0.9"}, aux["10.7.0.0/24"])
		assert.Equal(t, 3, len(ranges[0]))
		assert.Equal(t, "10.7.0.4", ranges[0][0].RangeEnd)
		assert.Equal(t, "10.7.0.6", ranges[0][1].RangeStart)
		assert.Equal(t, "10.7.0.8", ranges[0][1].RangeEnd)
		assert.Equal(t, "10.7.0.10", ranges[0][2].RangeStart)
	})

	t.Run("an aux-address in a subnet filtered out by disabled IPv6 errors", func(t *testing.T) {
		// The fd00:7::/64 subnet is skipped because ipv6 is false, so its
		// aux-address matches nothing and is reported, not silently dropped.
		_, _, _, err := pairIPAMRanges(parse(t, "10.7.0.0/24", "fd00:7::/64"), nil, nil, map[string]string{"v6": "fd00:7::9"}, false)
		assert.ErrorContains(t, err, "no matching subnet for aux-address fd00:7::9")
	})
}
