//go:build freebsd || linux

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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/defaults"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
	"github.com/containerd/nerdctl/v2/pkg/systemutil"
	"github.com/mitchellh/mapstructure"
	"github.com/vishvananda/netlink"
)

const (
	DefaultNetworkName = "bridge"
	DefaultCIDR        = "10.4.0.0/24"
	DefaultIPAMDriver  = "host-local"

	// When creating non-default network without passing in `--subnet` option,
	// nerdctl assigns subnet address for the creation starting from `StartingCIDR`
	// This prevents subnet address overlapping with `DefaultCIDR` used by the default network
	StartingCIDR = "10.4.1.0/24"
)

func (n *NetworkConfig) subnets() []*net.IPNet {
	var subnets []*net.IPNet
	if len(n.Plugins) > 0 && n.Plugins[0].Network.Type == "bridge" {
		var bridge bridgeConfig
		if err := json.Unmarshal(n.Plugins[0].Bytes, &bridge); err != nil {
			return subnets
		}
		if bridge.IPAM["type"] != "host-local" {
			return subnets
		}
		var ipam hostLocalIPAMConfig
		if err := mapstructure.Decode(bridge.IPAM, &ipam); err != nil {
			return subnets
		}
		for _, irange := range ipam.Ranges {
			if len(irange) > 0 {
				_, subnet, err := net.ParseCIDR(irange[0].Subnet)
				if err != nil {
					continue
				}
				subnets = append(subnets, subnet)
			}
		}
	}
	return subnets
}

func (n *NetworkConfig) clean() error {
	// Remove the bridge network interface on the host.
	if len(n.Plugins) > 0 && n.Plugins[0].Network.Type == "bridge" {
		var bridge bridgeConfig
		if err := json.Unmarshal(n.Plugins[0].Bytes, &bridge); err != nil {
			return err
		}
		return removeBridgeNetworkInterface(bridge.BrName)
	}
	return nil
}

func (e *CNIEnv) generateCNIPlugins(driver string, name string, ipam map[string]interface{}, opts map[string]string, ipv6 bool) ([]CNIPlugin, error) {
	var (
		plugins []CNIPlugin
		err     error
	)
	switch driver {
	case "bridge":
		mtu := 0
		iPMasq := true
		for opt, v := range opts {
			switch opt {
			case "mtu", "com.docker.network.driver.mtu":
				mtu, err = ParseMTU(v)
				if err != nil {
					return nil, err
				}
			case "ip-masq", "com.docker.network.bridge.enable_ip_masquerade":
				iPMasq, err = strconv.ParseBool(v)
				if err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("unsupported %q network option %q", driver, opt)
			}
		}
		var bridge *bridgeConfig
		if name == DefaultNetworkName {
			bridge = newBridgePlugin("nerdctl0")
		} else {
			bridge = newBridgePlugin("br-" + networkID(name)[:12])
		}
		bridge.MTU = mtu
		bridge.IPAM = ipam
		bridge.IsGW = true
		bridge.IPMasq = iPMasq
		bridge.HairpinMode = true
		if ipv6 {
			bridge.Capabilities["ips"] = true
		}
		plugins = []CNIPlugin{bridge, newPortMapPlugin(), newFirewallPlugin(), newTuningPlugin()}
		plugins = fixUpIsolation(e, name, plugins)
	case "macvlan", "ipvlan":
		mtu := 0
		mode := ""
		master := ""
		for opt, v := range opts {
			switch opt {
			case "mtu", "com.docker.network.driver.mtu":
				mtu, err = ParseMTU(v)
				if err != nil {
					return nil, err
				}
			case "mode", "macvlan_mode", "ipvlan_mode":
				if driver == "macvlan" && opt != "ipvlan_mode" {
					if !strutil.InStringSlice([]string{"bridge"}, v) {
						return nil, fmt.Errorf("unknown macvlan mode %q", v)
					}
				} else if driver == "ipvlan" && opt != "macvlan_mode" {
					if !strutil.InStringSlice([]string{"l2", "l3"}, v) {
						return nil, fmt.Errorf("unknown ipvlan mode %q", v)
					}
				} else {
					return nil, fmt.Errorf("unsupported %q network option %q", driver, opt)
				}
				mode = v
			case "parent":
				master = v
			default:
				return nil, fmt.Errorf("unsupported %q network option %q", driver, opt)
			}
		}
		vlan := newVLANPlugin(driver)
		vlan.MTU = mtu
		vlan.Master = master
		vlan.Mode = mode
		vlan.IPAM = ipam
		if ipv6 {
			vlan.Capabilities["ips"] = true
		}
		plugins = []CNIPlugin{vlan}
	default:
		return nil, fmt.Errorf("unsupported cni driver %q", driver)
	}
	return plugins, nil
}

func (e *CNIEnv) generateIPAM(driver string, subnets []string, gatewayStr, ipRangeStr string, opts map[string]string, ipv6 bool) (map[string]interface{}, error) {
	var ipamConfig interface{}
	switch driver {
	case "default", "host-local":
		ipamConf := newHostLocalIPAMConfig()
		ipamConf.Routes = []IPAMRoute{
			{Dst: "0.0.0.0/0"},
		}
		ranges, findIPv4, err := e.parseIPAMRanges(subnets, gatewayStr, ipRangeStr, ipv6)
		if err != nil {
			return nil, err
		}
		ipamConf.Ranges = append(ipamConf.Ranges, ranges...)
		if !findIPv4 {
			ranges, _, _ = e.parseIPAMRanges([]string{""}, gatewayStr, ipRangeStr, ipv6)
			ipamConf.Ranges = append(ipamConf.Ranges, ranges...)
		}
		ipamConfig = ipamConf
	case "dhcp":
		ipamConf := newDHCPIPAMConfig()
		ipamConf.DaemonSocketPath = filepath.Join(defaults.CNIRuntimeDir(), "dhcp.sock")
		// TODO: support IPAM options for dhcp
		if err := systemutil.IsSocketAccessible(ipamConf.DaemonSocketPath); err != nil {
			log.L.Warnf("cannot access dhcp socket %q (hint: try running with `dhcp daemon --socketpath=%s &` in CNI_PATH to launch the dhcp daemon)", ipamConf.DaemonSocketPath, ipamConf.DaemonSocketPath)
		}
		ipamConfig = ipamConf
	default:
		return nil, fmt.Errorf("unsupported ipam driver %q", driver)
	}

	ipam, err := structToMap(ipamConfig)
	if err != nil {
		return nil, err
	}
	return ipam, nil
}

func (e *CNIEnv) parseIPAMRanges(subnets []string, gateway, ipRange string, ipv6 bool) ([][]IPAMRange, bool, error) {
	findIPv4 := false
	ranges := make([][]IPAMRange, 0, len(subnets))
	for i := range subnets {
		subnet, err := e.parseSubnet(subnets[i])
		if err != nil {
			return nil, findIPv4, err
		}
		// if ipv6 flag is not set, subnets of ipv6 should be excluded
		if !ipv6 && subnet.IP.To4() == nil {
			continue
		}
		if !findIPv4 && subnet.IP.To4() != nil {
			findIPv4 = true
		}
		ipamRange, err := parseIPAMRange(subnet, gateway, ipRange)
		if err != nil {
			return nil, findIPv4, err
		}
		ranges = append(ranges, []IPAMRange{*ipamRange})
	}
	return ranges, findIPv4, nil
}

func fixUpIsolation(e *CNIEnv, name string, plugins []CNIPlugin) []CNIPlugin {
	isolationPath := filepath.Join(e.Path, "isolation")
	if _, err := exec.LookPath(isolationPath); err == nil {
		// the warning is suppressed for DefaultNetworkName (because multi-bridge networking is not involved)
		if name != DefaultNetworkName {
			log.L.Warnf(`network %q: Using the deprecated CNI "isolation" plugin instead of CNI "firewall" plugin (>= 1.1.0) ingressPolicy.
To dismiss this warning, uninstall %q and install CNI "firewall" plugin (>= 1.1.0) from https://github.com/containernetworking/plugins`,
				name, isolationPath)
		}
		plugins = append(plugins, newIsolationPlugin())
		for _, f := range plugins {
			if x, ok := f.(*firewallConfig); ok {
				if name != DefaultNetworkName {
					log.L.Warnf("network %q: Unsetting firewall ingressPolicy %q (because using the deprecated \"isolation\" plugin)", name, x.IngressPolicy)
				}
				x.IngressPolicy = ""
			}
		}
	} else if name != DefaultNetworkName {
		firewallPath := filepath.Join(e.Path, "firewall")
		ok, err := firewallPluginGEQ110(firewallPath)
		if err != nil {
			log.L.WithError(err).Warnf("Failed to detect whether %q is newer than v1.1.0", firewallPath)
		}
		if !ok {
			log.L.Warnf("To isolate bridge networks, CNI plugin \"firewall\" (>= 1.1.0) needs to be installed in CNI_PATH (%q), see https://github.com/containernetworking/plugins",
				e.Path)
		}
	}

	return plugins
}

func firewallPluginGEQ110(firewallPath string) (bool, error) {
	// TODO: guess true by default in 2023
	guessed := false

	// Parse the stderr (NOT stdout) of `firewall`, such as "CNI firewall plugin v1.1.0\n", or "CNI firewall plugin version unknown\n"
	//
	// We do NOT set `CNI_COMMAND=VERSION` here, because the CNI "VERSION" command reports the version of the CNI spec,
	// not the version of the firewall plugin implementation.
	//
	// ```
	// $ /opt/cni/bin/firewall
	// CNI firewall plugin v1.1.0
	// $ CNI_COMMAND=VERSION /opt/cni/bin/firewall
	// {"cniVersion":"1.0.0","supportedVersions":["0.4.0","1.0.0"]}
	// ```
	//
	cmd := exec.Command(firewallPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		err = fmt.Errorf("failed to run %v: %w (stdout=%q, stderr=%q)", cmd.Args, err, stdout.String(), stderr.String())
		return guessed, err
	}

	ver, err := guessFirewallPluginVersion(stderr.String()) // NOT stdout
	if err != nil {
		return guessed, fmt.Errorf("failed to guess the version of %q: %w", firewallPath, err)
	}
	ver110 := semver.MustParse("v1.1.0")
	return ver.GreaterThan(ver110) || ver.Equal(ver110), nil
}

// guesssFirewallPluginVersion guess the version of the CNI firewall plugin (not the version of the implemented CNI spec).
//
// stderr is like "CNI firewall plugin v1.1.0\n", or "CNI firewall plugin version unknown\n"
func guessFirewallPluginVersion(stderr string) (*semver.Version, error) {
	const prefix = "CNI firewall plugin "
	lines := strings.Split(stderr, "\n")
	for i, l := range lines {
		trimmed := strings.TrimPrefix(l, prefix)
		if trimmed == l { // l does not have the expected prefix
			continue
		}
		// trimmed is like "v1.1.1", "v1.1.0", ..., "v0.8.0", or "version unknown"
		ver, err := semver.NewVersion(trimmed)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %q (line %d of stderr=%q) as a semver: %w", trimmed, i+1, stderr, err)
		}
		return ver, nil
	}
	return nil, fmt.Errorf("stderr %q does not have any line that starts with %q", stderr, prefix)
}

func removeBridgeNetworkInterface(netIf string) error {
	link, err := netlink.LinkByName(netIf)
	if err == nil {
		if err := netlink.LinkDel(link); err != nil {
			return fmt.Errorf("failed to remove network interface %s: %v", netIf, err)
		}
	}
	return nil
}
