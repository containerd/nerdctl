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
	"context"
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

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/lockutil"
	"github.com/containerd/nerdctl/v2/pkg/netutil/nettype"
	subnetutil "github.com/containerd/nerdctl/v2/pkg/netutil/subnet"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
	"github.com/containernetworking/cni/libcni"
)

type CNIEnv struct {
	Path        string
	NetconfPath string
	Namespace   string
}

type CNIEnvOpt func(e *CNIEnv) error

func UsedNetworks(ctx context.Context, client *containerd.Client) (map[string][]string, error) {
	nsService := client.NamespaceService()
	nsList, err := nsService.List(ctx)
	if err != nil {
		return nil, err
	}
	used := make(map[string][]string)
	for _, ns := range nsList {
		nsCtx := namespaces.WithNamespace(ctx, ns)
		containers, err := client.Containers(nsCtx)
		if err != nil {
			return nil, err
		}
		nsUsedN, err := namespaceUsedNetworks(nsCtx, containers)
		if err != nil {
			return nil, err
		}

		// merge
		for k, v := range nsUsedN {
			if value, ok := used[k]; ok {
				used[k] = append(value, v...)
			} else {
				used[k] = v
			}
		}
	}
	return used, nil
}

func namespaceUsedNetworks(ctx context.Context, containers []containerd.Container) (map[string][]string, error) {
	used := make(map[string][]string)
	for _, c := range containers {
		// Only tasks under the ctx namespace can be obtained here
		task, err := c.Task(ctx, nil)
		if err != nil {
			if errdefs.IsNotFound(err) {
				log.G(ctx).Debugf("task not found - likely container %q was removed", c.ID())
				continue
			}
			return nil, err
		}
		status, err := task.Status(ctx)
		if err != nil {
			if errdefs.IsNotFound(err) {
				log.G(ctx).Debugf("task not found - likely container %q was removed", c.ID())
				continue
			}
			return nil, err
		}
		switch status.Status {
		case containerd.Paused, containerd.Running:
		default:
			continue
		}
		l, err := c.Labels(ctx)
		if err != nil {
			if errdefs.IsNotFound(err) {
				log.G(ctx).Debugf("container %q is gone", c.ID())
				continue
			}
			return nil, err
		}
		networkJSON, ok := l[labels.Networks]
		if !ok {
			continue
		}
		var networks []string
		if err := json.Unmarshal([]byte(networkJSON), &networks); err != nil {
			return nil, err
		}
		netType, err := nettype.Detect(networks)
		if err != nil {
			return nil, err
		}
		if netType != nettype.CNI {
			continue
		}
		for _, n := range networks {
			used[n] = append(used[n], c.ID())
		}
	}
	return used, nil
}

func WithDefaultNetwork() CNIEnvOpt {
	return func(e *CNIEnv) error {
		return e.ensureDefaultNetworkConfig()
	}
}

func WithNamespace(namespace string) CNIEnvOpt {
	return func(e *CNIEnv) error {
		if err := os.MkdirAll(filepath.Join(e.NetconfPath, namespace), 0755); err != nil {
			return err
		}
		e.Namespace = namespace
		return nil
	}
}

func NewCNIEnv(cniPath, cniConfPath string, opts ...CNIEnvOpt) (*CNIEnv, error) {
	e := CNIEnv{
		Path:        cniPath,
		NetconfPath: cniConfPath,
	}
	if err := os.MkdirAll(e.NetconfPath, 0755); err != nil {
		return nil, err
	}

	for _, o := range opts {
		if err := o(&e); err != nil {
			return nil, err
		}
	}

	return &e, nil
}

func (e *CNIEnv) NetworkList() ([]*NetworkConfig, error) {
	return e.networkConfigList()
}

func (e *CNIEnv) NetworkMap() (map[string]*NetworkConfig, error) { //nolint:revive
	networks, err := e.networkConfigList()
	if err != nil {
		return nil, err
	}

	m := make(map[string]*NetworkConfig, len(networks))
	for _, n := range networks {
		if original, exists := m[n.Name]; exists {
			log.L.Warnf("duplicate network name %q, %#v will get superseded by %#v", n.Name, original, n)
		}
		m[n.Name] = n
		if n.NerdctlID != nil {
			id := *n.NerdctlID
			m[id] = n
			if len(id) > 12 {
				id = id[:12]
				m[id] = n
			}
		}
	}
	return m, nil
}

func (e *CNIEnv) FilterNetworks(filterf func(*NetworkConfig) bool) ([]*NetworkConfig, error) {
	networkConfigs, err := e.networkConfigList()
	if err != nil {
		return nil, err
	}
	result := []*NetworkConfig{}
	for _, networkConfig := range networkConfigs {
		if filterf(networkConfig) {
			result = append(result, networkConfig)
		}
	}
	return result, nil
}

func (e *CNIEnv) getConfigPathForNetworkName(netName string) string {
	if netName == DefaultNetworkName || e.Namespace == "" {
		return filepath.Join(e.NetconfPath, "nerdctl-"+netName+".conflist")
	}
	return filepath.Join(e.NetconfPath, e.Namespace, "nerdctl-"+netName+".conflist")
}

func (e *CNIEnv) usedSubnets() ([]*net.IPNet, error) {
	usedSubnets, err := subnetutil.GetLiveNetworkSubnets()
	if err != nil {
		return nil, err
	}
	networkConfigs, err := e.networkConfigList()
	if err != nil {
		return nil, err
	}
	for _, net := range networkConfigs {
		usedSubnets = append(usedSubnets, net.subnets()...)
	}
	return usedSubnets, nil
}

type NetworkConfig struct {
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

func (e *CNIEnv) CreateNetwork(opts types.NetworkCreateOptions) (*NetworkConfig, error) { //nolint:revive
	var net *NetworkConfig
	netMap, err := e.NetworkMap()
	if err != nil {
		return nil, err
	}

	if _, ok := netMap[opts.Name]; ok {
		return nil, errdefs.ErrAlreadyExists
	}

	fn := func() error {
		ipam, err := e.generateIPAM(opts.IPAMDriver, opts.Subnets, opts.Gateway, opts.IPRange, opts.IPAMOptions, opts.IPv6)
		if err != nil {
			return err
		}
		plugins, err := e.generateCNIPlugins(opts.Driver, opts.Name, ipam, opts.Options, opts.IPv6)
		if err != nil {
			return err
		}
		net, err = e.generateNetworkConfig(opts.Name, opts.Labels, plugins)
		if err != nil {
			return err
		}
		return e.writeNetworkConfig(net)
	}
	err = lockutil.WithDirLock(e.NetconfPath, fn)
	if err != nil {
		return nil, err
	}
	return net, nil
}

func (e *CNIEnv) RemoveNetwork(net *NetworkConfig) error {
	fn := func() error {
		if err := os.RemoveAll(net.File); err != nil {
			return err
		}
		return net.clean()
	}
	return lockutil.WithDirLock(e.NetconfPath, fn)
}

// GetDefaultNetworkConfig checks whether the default network exists
// by first searching for if any network bears the `labels.NerdctlDefaultNetwork`
// label, or falls back to checking whether any network bears the
// `DefaultNetworkName` name.
func (e *CNIEnv) GetDefaultNetworkConfig() (*NetworkConfig, error) {
	// Search for networks bearing the `labels.NerdctlDefaultNetwork` label.
	defaultLabelFilterF := func(nc *NetworkConfig) bool {
		if nc.NerdctlLabels == nil {
			return false
		} else if _, ok := (*nc.NerdctlLabels)[labels.NerdctlDefaultNetwork]; ok {
			return true
		}
		return false
	}
	labelMatches, err := e.FilterNetworks(defaultLabelFilterF)
	if err != nil {
		return nil, err
	}
	if len(labelMatches) >= 1 {
		if len(labelMatches) > 1 {
			log.L.Warnf("returning the first network bearing the %q label out of the multiple found: %#v", labels.NerdctlDefaultNetwork, labelMatches)
		}
		return labelMatches[0], nil
	}

	// Search for networks bearing the DefaultNetworkName.
	defaultNameFilterF := func(nc *NetworkConfig) bool {
		return nc.Name == DefaultNetworkName
	}
	nameMatches, err := e.FilterNetworks(defaultNameFilterF)
	if err != nil {
		return nil, err
	}
	if len(nameMatches) >= 1 {
		if len(nameMatches) > 1 {
			log.L.Warnf("returning the first network bearing the %q default network name out of the multiple found: %#v", DefaultNetworkName, nameMatches)
		}

		// Warn the user if the default network was not created by nerdctl.
		match := nameMatches[0]
		_, statErr := os.Stat(e.getConfigPathForNetworkName(DefaultNetworkName))
		if match.NerdctlID == nil || statErr != nil {
			log.L.Warnf("default network named %q does not have an internal nerdctl ID or nerdctl-managed config file, it was most likely NOT created by nerdctl", DefaultNetworkName)
		}

		return nameMatches[0], nil
	}

	return nil, nil
}

func (e *CNIEnv) ensureDefaultNetworkConfig() error {
	defaultNet, err := e.GetDefaultNetworkConfig()
	if err != nil {
		return fmt.Errorf("failed to check for default network: %s", err)
	}
	if defaultNet == nil {
		if err := e.createDefaultNetworkConfig(); err != nil {
			return fmt.Errorf("failed to create default network: %s", err)
		}
	}
	return nil
}

func (e *CNIEnv) createDefaultNetworkConfig() error {
	filename := e.getConfigPathForNetworkName(DefaultNetworkName)
	if _, err := os.Stat(filename); err == nil {
		return fmt.Errorf("already found existing network config at %q, cannot create new network named %q", filename, DefaultNetworkName)
	}
	opts := types.NetworkCreateOptions{
		Name:       DefaultNetworkName,
		Driver:     DefaultNetworkName,
		Subnets:    []string{DefaultCIDR},
		IPAMDriver: "default",
		Labels:     []string{fmt.Sprintf("%s=true", labels.NerdctlDefaultNetwork)},
	}
	_, err := e.CreateNetwork(opts)
	if err != nil && !errdefs.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// generateNetworkConfig creates NetworkConfig.
// generateNetworkConfig does not fill "File" field.
func (e *CNIEnv) generateNetworkConfig(name string, labels []string, plugins []CNIPlugin) (*NetworkConfig, error) {
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
	return &NetworkConfig{
		NetworkConfigList: l,
		NerdctlID:         &id,
		NerdctlLabels:     &labelsMap,
		File:              "",
	}, nil
}

// writeNetworkConfig writes NetworkConfig file to cni config path.
func (e *CNIEnv) writeNetworkConfig(net *NetworkConfig) error {
	filename := e.getConfigPathForNetworkName(net.Name)
	if _, err := os.Stat(filename); err == nil {
		return errdefs.ErrAlreadyExists
	}
	return os.WriteFile(filename, net.Bytes, 0644)
}

// networkConfigList loads config from dir if dir exists.
func (e *CNIEnv) networkConfigList() ([]*NetworkConfig, error) {
	l := []*NetworkConfig{}
	common, err := libcni.ConfFiles(e.NetconfPath, []string{".conf", ".conflist", ".json"})
	if err != nil {
		return nil, err
	}
	namespaced := []string{}
	if e.Namespace != "" {
		namespaced, err = libcni.ConfFiles(filepath.Join(e.NetconfPath, e.Namespace), []string{".conf", ".conflist", ".json"})
		if err != nil {
			return nil, err
		}
	}
	fileNames := append(common, namespaced...)
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
		l = append(l, &NetworkConfig{
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
		_, defaultSubnet, _ := net.ParseCIDR(StartingCIDR)
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
