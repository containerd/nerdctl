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
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/containernetworking/cni/libcni"
)

type CNIEnv struct {
	Path        string
	NetconfPath string
	Networks    []*networkConfig
}

func NewCNIEnv(cniPath, cniConfPath string) (*CNIEnv, error) {
	e := CNIEnv{
		Path:        cniPath,
		NetconfPath: cniConfPath,
	}
	var err error
	err = e.ensureDefaultNetworkConfig()
	if err != nil {
		return nil, err
	}
	e.Networks, err = e.networkConfigList()
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (e *CNIEnv) NetworkMap() map[string]*networkConfig {
	m := make(map[string]*networkConfig, len(e.Networks))
	for _, n := range e.Networks {
		m[n.Name] = n
	}
	return m
}

type networkConfig struct {
	*libcni.NetworkConfigList
	NerdctlID     *int
	NerdctlLabels *map[string]string
	File          string
}

type cniNetworkConfig struct {
	CNIVersion string            `json:"cniVersion"`
	Name       string            `json:"name"`
	ID         int               `json:"nerdctlID"`
	Labels     map[string]string `json:"nerdctlLabels"`
	Plugins    []CNIPlugin       `json:"plugins"`
}

// GenerateNetworkConfig creates networkConfig.
// GenerateNetworkConfig does not fill "File" field.
func (e *CNIEnv) GenerateNetworkConfig(labels []string, id int, name string, plugins []CNIPlugin) (*networkConfig, error) {
	if e == nil || id < 0 || name == "" || len(plugins) == 0 {
		return nil, errdefs.ErrInvalidArgument
	}
	for _, f := range plugins {
		p := filepath.Join(e.Path, f.GetPluginType())
		if _, err := exec.LookPath(p); err != nil {
			return nil, fmt.Errorf("needs CNI plugin %q to be installed in CNI_PATH (%q), see https://github.com/containernetworking/plugins/releases: %w", f, e.Path, err)
		}
	}
	labelsMap := strutil.ConvertKVStringsToMap(labels)

	conf := &cniNetworkConfig{
		CNIVersion: "1.0.0",
		Name:       name,
		ID:         id,
		Labels:     labelsMap,
		Plugins:    plugins,
	}

	confJSON, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return nil, err
	}

	l, err := libcni.ConfListFromBytes(confJSON)
	if err != nil {
		return nil, err
	}
	return &networkConfig{
		NetworkConfigList: l,
		NerdctlID:         &id,
		NerdctlLabels:     &labelsMap,
		File:              "",
	}, nil
}

// WriteNetworkConfig writes networkConfig file to cni config path.
func (e *CNIEnv) WriteNetworkConfig(net *networkConfig) error {
	if err := os.MkdirAll(e.NetconfPath, 0755); err != nil {
		return err
	}
	filename := filepath.Join(e.NetconfPath, "nerdctl-"+net.Name+".conflist")
	if _, err := os.Stat(filename); err == nil {
		return errdefs.ErrAlreadyExists
	}
	if err := os.WriteFile(filename, net.Bytes, 0644); err != nil {
		return err
	}
	return nil
}

func (e *CNIEnv) ensureDefaultNetworkConfig() error {
	ipam, _ := GenerateIPAM("default", DefaultCIDR, "", "", nil)
	plugins, _ := e.GenerateCNIPlugins(DefaultNetworkName, DefaultID, DefaultNetworkName, ipam, nil)
	conf, err := e.GenerateNetworkConfig(nil, DefaultID, DefaultNetworkName, plugins)
	if err != nil {
		return err
	}
	err = e.WriteNetworkConfig(conf)
	if err != nil && !errdefs.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// networkConfigList loads config from dir if dir exists.
func (e *CNIEnv) networkConfigList() ([]*networkConfig, error) {
	l := []*networkConfig{}
	fileNames, err := libcni.ConfFiles(e.NetconfPath, []string{".conf", ".conflist", ".json"})
	if err != nil {
		return nil, err
	}
	sort.Strings(fileNames)
	for _, fileName := range fileNames {
		var lcl *libcni.NetworkConfigList
		if strings.HasSuffix(fileName, ".conflist") {
			lcl, err = libcni.ConfListFromFile(fileName)
			if err != nil {
				return nil, err
			}
		} else {
			lc, err := libcni.ConfFromFile(fileName)
			if err != nil {
				return nil, err
			}
			lcl, err = libcni.ConfListFromConf(lc)
			if err != nil {
				return nil, err
			}
		}
		l = append(l, &networkConfig{
			NetworkConfigList: lcl,
			NerdctlID:         nerdctlID(lcl.Bytes),
			NerdctlLabels:     nerdctlLabels(lcl.Bytes),
			File:              fileName,
		})
	}
	return l, nil
}

// AcquireNextID suggests the next ID.
func (e *CNIEnv) AcquireNextID() (int, error) {
	maxID := DefaultID
	for _, n := range e.Networks {
		if n.NerdctlID != nil && *n.NerdctlID > maxID {
			maxID = *n.NerdctlID
		}
	}
	nextID := maxID + 1
	return nextID, nil
}

func nerdctlID(b []byte) *int {
	type nerdctlConfigList struct {
		NerdctlID *int `json:"nerdctlID,omitempty"`
	}
	var ncl nerdctlConfigList
	if err := json.Unmarshal(b, &ncl); err != nil {
		// The network is managed outside nerdctl
		return nil
	}
	return ncl.NerdctlID
}

func nerdctlLabels(b []byte) *map[string]string {
	type nerdctlConfigList struct {
		NerdctlLabels *map[string]string `json:"nerdctlLabels,omitempty"`
	}
	var ncl nerdctlConfigList
	if err := json.Unmarshal(b, &ncl); err != nil {
		return nil
	}
	return ncl.NerdctlLabels
}

func GetBridgeName(id int) string {
	return fmt.Sprintf("nerdctl%d", id)
}

func parseIPAMRange(subnetStr, gatewayStr, ipRangeStr string) (*IPAMRange, error) {
	subnetIP, subnet, err := net.ParseCIDR(subnetStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse subnet %q", subnetStr)
	}
	if !subnet.IP.Equal(subnetIP) {
		return nil, fmt.Errorf("unexpected subnet %q, maybe you meant %q?", subnetStr, subnet.String())
	}

	var gateway, rangeStart, rangeEnd net.IP
	if gatewayStr != "" {
		gatewayIP := net.ParseIP(gatewayStr)
		if gatewayIP == nil {
			return nil, fmt.Errorf("failed to parse gateway %q", gatewayStr)
		}
		if !subnet.Contains(gatewayIP) {
			return nil, fmt.Errorf("no matching subnet %q for gateway %q", subnetStr, gatewayStr)
		}
		gateway = gatewayIP
	} else {
		gateway, _ = firstIPInSubnet(subnet)
	}

	res := &IPAMRange{
		Subnet:  subnet.String(),
		Gateway: gateway.String(),
	}

	if ipRangeStr != "" {
		_, ipRange, err := net.ParseCIDR(ipRangeStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ip-range %q", ipRangeStr)
		}
		rangeStart, _ = firstIPInSubnet(ipRange)
		rangeEnd, _ = lastIPInSubnet(ipRange)
		if !subnet.Contains(rangeStart) || !subnet.Contains(rangeEnd) {
			return nil, fmt.Errorf("no matching subnet %q for ip-range %q", subnetStr, ipRangeStr)
		}
		res.RangeStart = rangeStart.String()
		res.RangeEnd = rangeEnd.String()
		res.IPRange = ipRangeStr
	}

	return res, nil
}

// lastIPInSubnet gets the last IP in a subnet
// https://github.com/containers/podman/blob/v4.0.0-rc1/libpod/network/util/ip.go#L18
func lastIPInSubnet(addr *net.IPNet) (net.IP, error) {
	// re-parse to ensure clean network address
	_, cidr, err := net.ParseCIDR(addr.String())
	if err != nil {
		return nil, err
	}
	ones, bits := cidr.Mask.Size()
	if ones == bits {
		return cidr.IP, err
	}
	for i := range cidr.IP {
		cidr.IP[i] = cidr.IP[i] | ^cidr.Mask[i]
	}
	return cidr.IP, err
}

// firstIPInSubnet gets the first IP in a subnet
// https://github.com/containers/podman/blob/v4.0.0-rc1/libpod/network/util/ip.go#L36
func firstIPInSubnet(addr *net.IPNet) (net.IP, error) {
	// re-parse to ensure clean network address
	_, cidr, err := net.ParseCIDR(addr.String())
	if err != nil {
		return nil, err
	}
	ones, bits := cidr.Mask.Size()
	if ones == bits {
		return cidr.IP, err
	}
	cidr.IP[len(cidr.IP)-1]++
	return cidr.IP, err
}

// convert the struct to a map
func structToMap(in interface{}) (map[string]interface{}, error) {
	out := make(map[string]interface{})
	data, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ParseMTU parses the mtu option
func ParseMTU(mtu string) (int, error) {
	if mtu == "" {
		return 0, nil // default
	}
	m, err := strconv.Atoi(mtu)
	if err != nil {
		return 0, err
	}
	if m < 0 {
		return 0, fmt.Errorf("mtu %d is less than zero", m)
	}
	return m, nil
}
