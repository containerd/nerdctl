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
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"text/template"

	"gotest.tools/v3/assert"

	ncdefaults "github.com/containerd/nerdctl/v2/pkg/defaults"
	"github.com/containerd/nerdctl/v2/pkg/internal/filesystem"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

const testBridgeIP = "10.42.100.1/24" // nolint:unused

const preExistingNetworkConfigTemplate = `
{
    "cniVersion": "0.2.0",
    "name": "{{ .network_name }}",
    "type": "nat",
    "master": "Ethernet",
    "ipam": {
        "subnet": "{{ .subnet }}",
        "routes": [
            {
                "GW": "{{ .gateway }}"
            }
        ]
    },
    "capabilities": {
        "portMappings": true,
        "dns": true
    }
}
`

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
		_, subnet, _ := net.ParseCIDR(tc.subnet)
		got, err := parseIPAMRange(subnet, tc.gateway, tc.iprange)
		if tc.err != "" {
			assert.ErrorContains(t, err, tc.err)
		} else {
			assert.NilError(t, err)
			assert.DeepEqual(t, *tc.expected, *got)
		}
	}
}

func TestParseAuxAddresses(t *testing.T) {
	t.Parallel()
	type testCase struct {
		raw      []string
		expected map[string]string
		err      string
	}
	testCases := []testCase{
		{
			raw:      nil,
			expected: nil,
		},
		{
			raw:      []string{"router=10.1.100.5", "dns=10.1.100.6"},
			expected: map[string]string{"router": "10.1.100.5", "dns": "10.1.100.6"},
		},
		{
			// An empty name is allowed, matching Docker.
			raw:      []string{"=10.1.100.5"},
			expected: map[string]string{"": "10.1.100.5"},
		},
		{
			// An entry with no "=" has an empty IP and is dropped, matching Docker.
			raw:      []string{"10.1.100.5"},
			expected: nil,
		},
		{
			// A later value overrides an earlier one with the same name.
			raw:      []string{"a=10.1.100.5", "a=10.1.100.6"},
			expected: map[string]string{"a": "10.1.100.6"},
		},
		{
			raw:      []string{"v6=2001:db8::5"},
			expected: map[string]string{"v6": "2001:db8::5"},
		},
		{
			raw: []string{"bad=not-an-ip"},
			err: "invalid aux-address",
		},
	}
	for _, tc := range testCases {
		got, err := ParseAuxAddresses(tc.raw)
		if tc.err != "" {
			assert.ErrorContains(t, err, tc.err)
			continue
		}
		assert.NilError(t, err)
		assert.DeepEqual(t, tc.expected, got)
	}
}

func TestSplitIPAMRange(t *testing.T) {
	t.Parallel()
	ips := func(addrs ...string) []net.IP {
		out := make([]net.IP, len(addrs))
		for i, a := range addrs {
			out[i] = net.ParseIP(a)
		}
		return out
	}
	type testCase struct {
		name     string
		subnet   string
		base     *IPAMRange
		reserved []net.IP
		expected []IPAMRange
		err      string
	}
	testCases := []testCase{
		{
			name:     "no reservation leaves the range untouched",
			subnet:   "10.1.100.0/24",
			base:     &IPAMRange{Subnet: "10.1.100.0/24", Gateway: "10.1.100.1"},
			reserved: nil,
			expected: []IPAMRange{{Subnet: "10.1.100.0/24", Gateway: "10.1.100.1"}},
		},
		{
			name:     "a mid-subnet reservation splits the range in two",
			subnet:   "10.1.100.0/24",
			base:     &IPAMRange{Subnet: "10.1.100.0/24", Gateway: "10.1.100.1"},
			reserved: ips("10.1.100.5"),
			expected: []IPAMRange{
				{Subnet: "10.1.100.0/24", RangeStart: "10.1.100.1", RangeEnd: "10.1.100.4", Gateway: "10.1.100.1"},
				{Subnet: "10.1.100.0/24", RangeStart: "10.1.100.6", RangeEnd: "10.1.100.254", Gateway: "10.1.100.1"},
			},
		},
		{
			name:     "two reservations produce three sub-ranges",
			subnet:   "10.1.100.0/24",
			base:     &IPAMRange{Subnet: "10.1.100.0/24", Gateway: "10.1.100.1"},
			reserved: ips("10.1.100.6", "10.1.100.5"),
			expected: []IPAMRange{
				{Subnet: "10.1.100.0/24", RangeStart: "10.1.100.1", RangeEnd: "10.1.100.4", Gateway: "10.1.100.1"},
				{Subnet: "10.1.100.0/24", RangeStart: "10.1.100.7", RangeEnd: "10.1.100.254", Gateway: "10.1.100.1"},
			},
		},
		{
			// The gateway is the first usable address, so reserving the next one
			// leaves a gateway-only sub-range; host-local reserves the gateway, so
			// allocation still starts after the reservation.
			name:     "a reservation right after the gateway leaves a gateway-only range",
			subnet:   "10.1.100.0/24",
			base:     &IPAMRange{Subnet: "10.1.100.0/24", Gateway: "10.1.100.1"},
			reserved: ips("10.1.100.2"),
			expected: []IPAMRange{
				{Subnet: "10.1.100.0/24", RangeStart: "10.1.100.1", RangeEnd: "10.1.100.1", Gateway: "10.1.100.1"},
				{Subnet: "10.1.100.0/24", RangeStart: "10.1.100.3", RangeEnd: "10.1.100.254", Gateway: "10.1.100.1"},
			},
		},
		{
			name:     "a reservation inside an ip-range splits within its bounds",
			subnet:   "10.1.100.0/24",
			base:     &IPAMRange{Subnet: "10.1.100.0/24", Gateway: "10.1.100.1", IPRange: "10.1.100.0/28", RangeStart: "10.1.100.1", RangeEnd: "10.1.100.15"},
			reserved: ips("10.1.100.5"),
			expected: []IPAMRange{
				{Subnet: "10.1.100.0/24", RangeStart: "10.1.100.1", RangeEnd: "10.1.100.4", Gateway: "10.1.100.1", IPRange: "10.1.100.0/28"},
				{Subnet: "10.1.100.0/24", RangeStart: "10.1.100.6", RangeEnd: "10.1.100.15", Gateway: "10.1.100.1"},
			},
		},
		{
			name:     "a reservation outside the ip-range needs no split",
			subnet:   "10.1.100.0/24",
			base:     &IPAMRange{Subnet: "10.1.100.0/24", Gateway: "10.1.100.1", IPRange: "10.1.100.0/28", RangeStart: "10.1.100.1", RangeEnd: "10.1.100.15"},
			reserved: ips("10.1.100.200"),
			expected: []IPAMRange{
				{Subnet: "10.1.100.0/24", Gateway: "10.1.100.1", IPRange: "10.1.100.0/28", RangeStart: "10.1.100.1", RangeEnd: "10.1.100.15"},
			},
		},
		{
			// Reserving every usable address in the window leaves nothing to hand
			// out, which is an error rather than an empty range set.
			name:     "reservations leaving no allocatable address error",
			subnet:   "10.1.100.0/30",
			base:     &IPAMRange{Subnet: "10.1.100.0/30", Gateway: "10.1.100.1"},
			reserved: ips("10.1.100.1", "10.1.100.2"),
			err:      "leave no allocatable",
		},
		{
			name:     "an IPv6 reservation splits the range around it",
			subnet:   "2001:db8::/64",
			base:     &IPAMRange{Subnet: "2001:db8::/64", Gateway: "2001:db8::1"},
			reserved: ips("2001:db8::5"),
			// IPv6 has no broadcast, so the last address stays allocatable.
			expected: []IPAMRange{
				{Subnet: "2001:db8::/64", RangeStart: "2001:db8::1", RangeEnd: "2001:db8::4", Gateway: "2001:db8::1"},
				{Subnet: "2001:db8::/64", RangeStart: "2001:db8::6", RangeEnd: "2001:db8::ffff:ffff:ffff:ffff", Gateway: "2001:db8::1"},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, subnet, err := net.ParseCIDR(tc.subnet)
			assert.NilError(t, err)
			got, err := splitIPAMRange(subnet, tc.base, tc.reserved)
			if tc.err != "" {
				assert.ErrorContains(t, err, tc.err)
				return
			}
			assert.NilError(t, err)
			assert.DeepEqual(t, tc.expected, got)
		})
	}
}

// Tests whether nerdctl properly creates the default network when required.
// Note that this test will require a CNI driver bearing the same name as
// the type of the default network. (denoted by netutil.DefaultNetworkName,
// which is used as both the name of the default network and its Driver)
// nolint:unused
func testDefaultNetworkCreation(t *testing.T) {
	// To prevent subnet collisions when attempting to recreate the default network
	// in the isolated CNI config dir we'll be using, we must first delete
	// the network in the default CNI config dir.
	defaultCniEnv := CNIEnv{
		Path:        ncdefaults.CNIPath(),
		NetconfPath: ncdefaults.CNINetConfPath(),
	}
	defaultNet, err := defaultCniEnv.GetDefaultNetworkConfig()
	assert.NilError(t, err)
	if defaultNet != nil {
		assert.NilError(t, defaultCniEnv.RemoveNetwork(defaultNet))
	}

	// We create a tempdir for the CNI conf path to ensure an empty env for this test.
	cniConfTestDir := t.TempDir()
	cniEnv := CNIEnv{
		Path:        ncdefaults.CNIPath(),
		NetconfPath: cniConfTestDir,
	}
	// Ensure no default network config is not present.
	defaultNetConf, err := cniEnv.GetDefaultNetworkConfig()
	assert.NilError(t, err)
	assert.Assert(t, defaultNetConf == nil)

	// Attempt to create the default network.
	err = cniEnv.ensureDefaultNetworkConfig("")
	assert.NilError(t, err)

	// Ensure default network config is present now.
	defaultNetConf, err = cniEnv.GetDefaultNetworkConfig()
	assert.NilError(t, err)
	assert.Assert(t, defaultNetConf != nil)

	// Check network config file present.
	stat, err := os.Stat(defaultNetConf.File)
	assert.NilError(t, err)
	firstConfigModTime := stat.ModTime()

	// Check default network label present.
	assert.Assert(t, defaultNetConf.NerdctlLabels != nil)
	lstr, ok := (*defaultNetConf.NerdctlLabels)[labels.NerdctlDefaultNetwork]
	assert.Assert(t, ok)
	boolv, err := strconv.ParseBool(lstr)
	assert.NilError(t, err)
	assert.Assert(t, boolv)

	// Ensure network isn't created twice or accidentally re-created.
	err = cniEnv.ensureDefaultNetworkConfig("")
	assert.NilError(t, err)

	// Check for any other network config files.
	files := []os.FileInfo{}
	walkF := func(p string, info os.FileInfo, err error) error {
		files = append(files, info)
		return nil
	}
	err = filepath.Walk(cniConfTestDir, walkF)
	assert.NilError(t, err)
	assert.Equal(t, len(files), 3) // files[0] is the entry for '.', files[1] is the lock
	assert.Assert(t, filepath.Join(cniConfTestDir, files[2].Name()) == defaultNetConf.File)
	assert.Assert(t, firstConfigModTime.Equal(files[2].ModTime()))
}

// Tests whether nerdctl properly creates the default network
// with a custom bridge IP and subnet.
// nolint:unused
func testDefaultNetworkCreationWithBridgeIP(t *testing.T) {
	// To prevent subnet collisions when attempting to recreate the default network
	// in the isolated CNI config dir we'll be using, we must first delete
	// the network in the default CNI config dir.
	defaultCniEnv := CNIEnv{
		Path:        ncdefaults.CNIPath(),
		NetconfPath: ncdefaults.CNINetConfPath(),
	}
	defaultNet, err := defaultCniEnv.GetDefaultNetworkConfig()
	assert.NilError(t, err)
	if defaultNet != nil {
		assert.NilError(t, defaultCniEnv.RemoveNetwork(defaultNet))
	}

	// We create a tempdir for the CNI conf path to ensure an empty env for this test.
	cniConfTestDir := t.TempDir()
	cniEnv := CNIEnv{
		Path:        ncdefaults.CNIPath(),
		NetconfPath: cniConfTestDir,
	}
	// Ensure no default network config is not present.
	defaultNetConf, err := cniEnv.GetDefaultNetworkConfig()
	assert.NilError(t, err)
	assert.Assert(t, defaultNetConf == nil)

	// Attempt to create the default network with a test bridgeIP
	err = cniEnv.ensureDefaultNetworkConfig(testBridgeIP)
	assert.NilError(t, err)

	// Ensure default network config is present now.
	defaultNetConf, err = cniEnv.GetDefaultNetworkConfig()
	assert.NilError(t, err)
	assert.Assert(t, defaultNetConf != nil)

	// Check network config file present.
	stat, err := os.Stat(defaultNetConf.File)
	assert.NilError(t, err)
	firstConfigModTime := stat.ModTime()

	// Check default network label present.
	assert.Assert(t, defaultNetConf.NerdctlLabels != nil)
	lstr, ok := (*defaultNetConf.NerdctlLabels)[labels.NerdctlDefaultNetwork]
	assert.Assert(t, ok)
	boolv, err := strconv.ParseBool(lstr)
	assert.NilError(t, err)
	assert.Assert(t, boolv)

	// Check bridge IP is set.
	assert.Assert(t, defaultNetConf.Plugins != nil)
	assert.Assert(t, len(defaultNetConf.Plugins) > 0)
	bridgePlugin := defaultNetConf.Plugins[0]
	var bridgeConfig struct {
		Type   string `json:"type"`
		Bridge string `json:"bridge"`
		IPAM   struct {
			Ranges [][]struct {
				Gateway string `json:"gateway"`
				Subnet  string `json:"subnet"`
			} `json:"ranges"`
			Routes []struct {
				Dst string `json:"dst"`
			} `json:"routes"`
			Type string `json:"type"`
		} `json:"ipam"`
	}

	err = json.Unmarshal(bridgePlugin.Bytes, &bridgeConfig)
	if err != nil {
		t.Fatalf("Failed to parse bridge plugin config: %v", err)
	}

	// Assert on bridge plugin configuration
	assert.Equal(t, "bridge", bridgeConfig.Type)
	// Assert on IPAM configuration
	assert.Equal(t, "10.42.100.1", bridgeConfig.IPAM.Ranges[0][0].Gateway)
	assert.Equal(t, "10.42.100.0/24", bridgeConfig.IPAM.Ranges[0][0].Subnet)
	assert.Equal(t, "0.0.0.0/0", bridgeConfig.IPAM.Routes[0].Dst)
	assert.Equal(t, "host-local", bridgeConfig.IPAM.Type)

	// Ensure network isn't created twice or accidentally re-created.
	err = cniEnv.ensureDefaultNetworkConfig(testBridgeIP)
	assert.NilError(t, err)

	// Check for any other network config files.
	files := []os.FileInfo{}
	walkF := func(p string, info os.FileInfo, err error) error {
		files = append(files, info)
		return nil
	}
	err = filepath.Walk(cniConfTestDir, walkF)
	assert.NilError(t, err)
	assert.Equal(t, len(files), 3) // files[0] is the entry for '.', files[1] is the lock
	assert.Assert(t, filepath.Join(cniConfTestDir, files[2].Name()) == defaultNetConf.File)
	assert.Assert(t, firstConfigModTime.Equal(files[2].ModTime()))
}

// Tests whether nerdctl skips the creation of the default network if a
// network bearing the default network name already exists in a
// non-nerdctl-managed network config file.
func TestNetworkWithDefaultNameAlreadyExists(t *testing.T) {
	// We create a tempdir for the CNI conf path to ensure an empty env for this test.
	cniConfTestDir := t.TempDir()
	cniEnv := CNIEnv{
		Path:        t.TempDir(), // irrelevant for this test
		NetconfPath: cniConfTestDir,
	}

	// Ensure no default network config is not present.
	defaultNetConf, err := cniEnv.GetDefaultNetworkConfig()
	assert.NilError(t, err)
	assert.Assert(t, defaultNetConf == nil)

	// Manually define and write a network config file.
	values := map[string]string{
		"network_name": DefaultNetworkName,
		"subnet":       "10.7.1.1/24",
		"gateway":      "10.7.1.1",
	}
	tpl, err := template.New("test").Parse(preExistingNetworkConfigTemplate)
	assert.NilError(t, err)
	buf := &bytes.Buffer{}
	assert.NilError(t, tpl.ExecuteTemplate(buf, "test", values))

	// Filename is irrelevant as long as it's not nerdctl's.
	testConfFile := filepath.Join(cniConfTestDir, fmt.Sprintf("%s.conf", testutil.Identifier(t)))
	err = filesystem.WriteFile(testConfFile, buf.Bytes(), 0600)
	assert.NilError(t, err)

	// Check network is detected.
	netConfs, err := cniEnv.NetworkList()
	assert.NilError(t, err)
	assert.Assert(t, len(netConfs) > 0)

	var listedDefaultNetConf *NetworkConfig
	for _, netConf := range netConfs {
		if netConf.Name == DefaultNetworkName {
			listedDefaultNetConf = netConf
			break
		}
	}
	assert.Assert(t, listedDefaultNetConf != nil)

	defaultNetConf, err = cniEnv.GetDefaultNetworkConfig()
	assert.NilError(t, err)
	assert.Assert(t, defaultNetConf != nil)
	assert.Assert(t, defaultNetConf.File == testConfFile)

	err = cniEnv.ensureDefaultNetworkConfig("")
	assert.NilError(t, err)

	netConfs, err = cniEnv.NetworkList()
	assert.NilError(t, err)
	defaultNamedNetworksFileDefinitions := []string{}
	for _, netConf := range netConfs {
		if netConf.Name == DefaultNetworkName {
			defaultNamedNetworksFileDefinitions = append(defaultNamedNetworksFileDefinitions, netConf.File)
		}
	}
	assert.Assert(t, len(defaultNamedNetworksFileDefinitions) == 1)
	assert.Assert(t, defaultNamedNetworksFileDefinitions[0] == testConfFile)
}

func TestFSExistsPropagatesStatError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cni-conf-root")
	assert.NilError(t, filesystem.WriteFile(path, nil, 0600))

	cniEnv := CNIEnv{
		NetconfPath: path,
	}

	exists, err := fsExists(&cniEnv, DefaultNetworkName)
	assert.Assert(t, !exists)
	assert.Assert(t, err != nil)
}

func TestListNetworksMatchIncludesPseudoNetworks(t *testing.T) {
	cniConfTestDir := t.TempDir()
	cniEnv := CNIEnv{
		Path:        t.TempDir(),
		NetconfPath: cniConfTestDir,
	}

	values := map[string]string{
		"network_name": "regular-network",
		"subnet":       "10.7.1.0/24",
		"gateway":      "10.7.1.1",
	}
	tpl, err := template.New("test").Parse(preExistingNetworkConfigTemplate)
	assert.NilError(t, err)
	buf := &bytes.Buffer{}
	assert.NilError(t, tpl.ExecuteTemplate(buf, "test", values))

	testConfFile := filepath.Join(cniConfTestDir, fmt.Sprintf("%s.conf", testutil.Identifier(t)))
	assert.NilError(t, filesystem.WriteFile(testConfFile, buf.Bytes(), 0600))

	matches, errs := cniEnv.ListNetworksMatch([]string{"host", "none", "regular-network"}, true)
	assert.Assert(t, len(errs) == 0)
	assert.Equal(t, len(matches["host"]), 1)
	assert.Equal(t, matches["host"][0].Name, "host")
	assert.Equal(t, len(matches["none"]), 1)
	assert.Equal(t, matches["none"][0].Name, "none")
	assert.Equal(t, len(matches["regular-network"]), 1)
	assert.Equal(t, matches["regular-network"][0].Name, "regular-network")
}
