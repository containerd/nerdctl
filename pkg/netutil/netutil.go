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
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/containerd/nerdctl/pkg/lockutil"
	subnetutil "github.com/containerd/nerdctl/pkg/netutil/subnet"
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
	if err := os.MkdirAll(e.NetconfPath, 0755); err != nil {
		return nil, err
	}
	if err := e.ensureDefaultNetworkConfig(); err != nil {
		return nil, err
	}
	networks, err := e.networkConfigList()
	if err != nil {
		return nil, err
	}
	e.Networks = networks
	return &e, nil
}

func (e *CNIEnv) NetworkMap() map[string]*networkConfig { //nolint:revive
	m := make(map[string]*networkConfig, len(e.Networks))
	for _, n := range e.Networks {
		m[n.Name] = n
	}
	return m
}

func (e *CNIEnv) usedSubnets() ([]*net.IPNet, error) {
	usedSubnets, err := subnetutil.GetLiveNetworkSubnets()
	if err != nil {
		return nil, err
	}
	for _, net := range e.Networks {
		usedSubnets = append(usedSubnets, net.subnets()...)
	}
	return usedSubnets, nil
}

type networkConfig struct {
	*libcni.NetworkConfigList
	NerdctlID     *string
	NerdctlLabels *map[string]string
	File          string
}

type cniNetworkConfig struct {
	CNIVersion string            `json:"cniVersion"`
	Name       string            `json:"name"`
	ID         string            `json:"nerdctlID"`
	Labels     map[string]string `json:"nerdctlLabels"`
	Plugins    []CNIPlugin       `json:"plugins"`
}

type CreateOptions struct {
	Name        string
	Driver      string
	Options     map[string]string
	IPAMDriver  string
	IPAMOptions map[string]string
	Subnet      string
	Gateway     string
	IPRange     string
	Labels      []string
}

func (e *CNIEnv) CreateNetwork(opts CreateOptions) (*networkConfig, error) { //nolint:revive
	var net *networkConfig
	if _, ok := e.NetworkMap()[opts.Name]; ok {
		return nil, errdefs.ErrAlreadyExists
	}

	fn := func() error {
		ipam, err := e.generateIPAM(opts.IPAMDriver, opts.Subnet, opts.Gateway, opts.IPRange, opts.IPAMOptions)
		if err != nil {
			return err
		}
		plugins, err := e.generateCNIPlugins(opts.Driver, opts.Name, ipam, opts.Options)
		if err != nil {
			return err
		}
		net, err = e.generateNetworkConfig(opts.Name, opts.Labels, plugins)
		if err != nil {
			return err
		}
		if err := e.writeNetworkConfig(net); err != nil {
			return err
		}
		return nil
	}
	err := lockutil.WithDirLock(e.NetconfPath, fn)
	if err != nil {
		return nil, err
	}
	return net, nil
}

func (e *CNIEnv) RemoveNetwork(net *networkConfig) error {
	fn := func() error {
		if err := os.RemoveAll(net.File); err != nil {
			return err
		}
		if err := net.clean(); err != nil {
			return err
		}
		return nil
	}
	return lockutil.WithDirLock(e.NetconfPath, fn)
}

func (e *CNIEnv) ensureDefaultNetworkConfig() error {
	filename := filepath.Join(e.NetconfPath, "nerdctl-"+DefaultNetworkName+".conflist")
	if _, err := os.Stat(filename); err == nil {
		return nil
	}
	opts := CreateOptions{
		Name:       DefaultNetworkName,
		Driver:     DefaultNetworkName,
		Subnet:     DefaultCIDR,
		IPAMDriver: "default",
	}
	_, err := e.CreateNetwork(opts)
	if err != nil && !errdefs.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// generateNetworkConfig creates networkConfig.
// generateNetworkConfig does not fill "File" field.
func (e *CNIEnv) generateNetworkConfig(name string, labels []string, plugins []CNIPlugin) (*networkConfig, error) {
	if name == "" || len(plugins) == 0 {
		return nil, errdefs.ErrInvalidArgument
	}
	for _, f := range plugins {
		p := filepath.Join(e.Path, f.GetPluginType())
		if _, err := exec.LookPath(p); err != nil {
			return nil, fmt.Errorf("needs CNI plugin %q to be installed in CNI_PATH (%q), see https://github.com/containernetworking/plugins/releases: %w", f.GetPluginType(), e.Path, err)
		}
	}
	id := networkID(name)
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

// writeNetworkConfig writes networkConfig file to cni config path.
func (e *CNIEnv) writeNetworkConfig(net *networkConfig) error {
	filename := filepath.Join(e.NetconfPath, "nerdctl-"+net.Name+".conflist")
	if _, err := os.Stat(filename); err == nil {
		return errdefs.ErrAlreadyExists
	}
	if err := os.WriteFile(filename, net.Bytes, 0644); err != nil {
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
		id, labels := nerdctlIDLabels(lcl.Bytes)
		l = append(l, &networkConfig{
			NetworkConfigList: lcl,
			NerdctlID:         id,
			NerdctlLabels:     labels,
			File:              fileName,
		})
	}
	return l, nil
}

func nerdctlIDLabels(b []byte) (*string, *map[string]string) {
	type idLabels struct {
		ID     *string            `json:"nerdctlID,omitempty"`
		Labels *map[string]string `json:"nerdctlLabels,omitempty"`
	}
	var idl idLabels
	if err := json.Unmarshal(b, &idl); err != nil {
		return nil, nil
	}
	return idl.ID, idl.Labels
}

func networkID(name string) string {
	hash := sha256.Sum256([]byte(name))
	return hex.EncodeToString(hash[:])
}

func (e *CNIEnv) parseSubnet(subnetStr string) (*net.IPNet, error) {
	usedSubnets, err := e.usedSubnets()
	if err != nil {
		return nil, err
	}
	if subnetStr == "" {
		_, defaultSubnet, _ := net.ParseCIDR(DefaultCIDR)
		subnet, err := subnetutil.GetFreeSubnet(defaultSubnet, usedSubnets)
		if err != nil {
			return nil, err
		}
		return subnet, nil
	}

	subnetIP, subnet, err := net.ParseCIDR(subnetStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse subnet %q", subnetStr)
	}
	if !subnet.IP.Equal(subnetIP) {
		return nil, fmt.Errorf("unexpected subnet %q, maybe you meant %q?", subnetStr, subnet.String())
	}
	if subnetutil.IntersectsWithNetworks(subnet, usedSubnets) {
		return nil, fmt.Errorf("subnet %s overlaps with other one on this address space", subnetStr)
	}
	return subnet, nil
}

func parseIPAMRange(subnet *net.IPNet, gatewayStr, ipRangeStr string) (*IPAMRange, error) {
	var gateway, rangeStart, rangeEnd net.IP
	if gatewayStr != "" {
		gatewayIP := net.ParseIP(gatewayStr)
		if gatewayIP == nil {
			return nil, fmt.Errorf("failed to parse gateway %q", gatewayStr)
		}
		if !subnet.Contains(gatewayIP) {
			return nil, fmt.Errorf("no matching subnet %q for gateway %q", subnet, gatewayStr)
		}
		gateway = gatewayIP
	} else {
		gateway, _ = subnetutil.FirstIPInSubnet(subnet)
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
		rangeStart, _ = subnetutil.FirstIPInSubnet(ipRange)
		rangeEnd, _ = subnetutil.LastIPInSubnet(ipRange)
		if !subnet.Contains(rangeStart) || !subnet.Contains(rangeEnd) {
			return nil, fmt.Errorf("no matching subnet %q for ip-range %q", subnet, ipRangeStr)
		}
		res.RangeStart = rangeStart.String()
		res.RangeEnd = rangeEnd.String()
		res.IPRange = ipRangeStr
	}

	return res, nil
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
