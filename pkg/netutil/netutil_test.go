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
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"text/template"

	"gotest.tools/v3/assert"

	ncdefaults "github.com/containerd/nerdctl/v2/pkg/defaults"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
)

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
			assert.Equal(t, *tc.expected, *got)
		}
	}
}

// Tests whether nerdctl properly creates the default network when required.
// Note that this test will require a CNI driver bearing the same name as
// the type of the default network. (denoted by netutil.DefaultNetworkName,
// which is used as both the name of the default network and its Driver)
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
	err = cniEnv.ensureDefaultNetworkConfig()
	assert.NilError(t, err)

	// Ensure no default network config is present now.
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
	err = cniEnv.ensureDefaultNetworkConfig()
	assert.NilError(t, err)

	// Check for any other network config files.
	files := []os.FileInfo{}
	walkF := func(p string, info os.FileInfo, err error) error {
		files = append(files, info)
		return nil
	}
	err = filepath.Walk(cniConfTestDir, walkF)
	assert.NilError(t, err)
	assert.Assert(t, len(files) == 2) // files[0] is the entry for '.'
	assert.Assert(t, filepath.Join(cniConfTestDir, files[1].Name()) == defaultNetConf.File)
	assert.Assert(t, firstConfigModTime == files[1].ModTime())
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
	err = os.WriteFile(testConfFile, buf.Bytes(), 0600)
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

	err = cniEnv.ensureDefaultNetworkConfig()
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
