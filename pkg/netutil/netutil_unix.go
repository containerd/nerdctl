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
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/go-viper/mapstructure/v2"
	"github.com/vishvananda/netlink"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/defaults"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
	"github.com/containerd/nerdctl/v2/pkg/systemutil"
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

func (e *CNIEnv) generateCNIPlugins(driver string, name string, ipam map[string]interface{}, opts map[string]string, ipv6 bool, internal bool) ([]CNIPlugin, error) {
	var (
		plugins []CNIPlugin
		err     error
	)
	switch driver {
	case "bridge":
		mtu := 0
		iPMasq := true
		icc := true
		for opt, v := range opts {
			switch opt {
			case "mtu", "com.docker.network.driver.mtu":
				mtu, err = parseMTU(v)
				if err != nil {
					return nil, err
				}
			case "ip-masq", "com.docker.network.bridge.enable_ip_masquerade":
				iPMasq, err = strconv.ParseBool(v)
				if err != nil {
					return nil, err
				}
			case "icc", "com.docker.network.bridge.enable_icc":
				icc, err = strconv.ParseBool(v)
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
		bridge.IsGW = !internal
		if internal {
			bridge.IPMasq = false
		} else {
			bridge.IPMasq = iPMasq
		}
		bridge.HairpinMode = true
		if ipv6 {
			bridge.Capabilities["ips"] = true
		}

		// Determine the appropriate firewall ingress policy based on icc setting
		ingressPolicy := "same-bridge" // Default policy
		firewallPath := filepath.Join(e.Path, "firewall")
		if !icc {
			// Check if firewall plugin supports the "isolated" policy (v1.7.1+)
			ok, err := FirewallPluginGEQVersion(firewallPath, "v1.7.1")
			if err != nil {
				log.L.WithError(err).Warnf("Failed to detect whether %q is newer than v1.7.1", firewallPath)
			} else if ok {
				ingressPolicy = "isolated"
			} else {
				log.L.Warnf("To use 'isolated' ingress policy, CNI plugin \"firewall\" (>= 1.7.1) needs to be installed in CNI_PATH (%q), see https://www.cni.dev/plugins/current/meta/firewall/", e.Path)
			}
		}

		if internal {
			plugins = []CNIPlugin{bridge, newFirewallPlugin(ingressPolicy), newTuningPlugin()}
		} else {
			plugins = []CNIPlugin{bridge, newPortMapPlugin(), newFirewallPlugin(ingressPolicy), newTuningPlugin()}
		}
		if name != DefaultNetworkName {
			ok, err := FirewallPluginGEQVersion(firewallPath, "v1.1.0")
			if err != nil {
				log.L.WithError(err).Warnf("Failed to detect whether %q is newer than v1.1.0", firewallPath)
			}
			if !ok {
				log.L.Warnf("To isolate bridge networks, CNI plugin \"firewall\" (>= 1.1.0) needs to be installed in CNI_PATH (%q), see https://github.com/containernetworking/plugins",
					e.Path)
			}
		}
	case "macvlan", "ipvlan":
		mtu := 0
		mode := ""
		master := ""
		for opt, v := range opts {
			switch opt {
			case "mtu", "com.docker.network.driver.mtu":
				mtu, err = parseMTU(v)
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

func (e *CNIEnv) generateIPAM(driver string, subnets []string, gateways []string, ipRanges []string, auxAddresses []string, opts map[string]string, ipv6, ipv4, internal bool) (map[string]interface{}, map[string]map[string]string, error) {
	var ipamConfig interface{}
	// auxBySubnet carries each subnet's reserved aux-addresses back to the caller
	// so they can be stored in a nerdctl label instead of the CNI config; it stays
	// nil for drivers other than host-local, which have no such reservation.
	var auxBySubnet map[string]map[string]string
	switch driver {
	case "default", "host-local":
		// Reserved auxiliary addresses are only meaningful for host-local, where
		// they are enforced by carving the reserved IPs out of the range below.
		aux, err := ParseAuxAddresses(auxAddresses)
		if err != nil {
			return nil, nil, err
		}
		ipamConf := newHostLocalIPAMConfig()
		if !internal {
			// An IPv6-only network has no IPv4 gateway, so its default route
			// must be the IPv6 one; otherwise host-local installs an IPv4
			// default route with no matching range.
			defaultRoute := "0.0.0.0/0"
			if !ipv4 {
				defaultRoute = "::/0"
			}
			ipamConf.Routes = []IPAMRoute{
				{Dst: defaultRoute},
			}
		}
		ranges, findIPv4, auxByNet, err := e.parseIPAMRanges(subnets, gateways, ipRanges, aux, ipv6)
		if err != nil {
			return nil, nil, err
		}
		auxBySubnet = auxByNet
		if !ipv4 && findIPv4 {
			return nil, nil, fmt.Errorf("--ipv4=false conflicts with an IPv4 subnet")
		}
		ipamConf.Ranges = append(ipamConf.Ranges, ranges...)
		if ipv4 && !findIPv4 {
			// The default IPv4 range uses a computed gateway and no ip-range;
			// any user-supplied gateway or ip-range belongs to an explicit subnet.
			// It also has no user subnet, so no aux-address can match it.
			// Skipped when IPv4 is disabled, leaving the network IPv6-only.
			ranges, _, _, _ = e.parseIPAMRanges([]string{""}, nil, nil, nil, ipv6)
			ipamConf.Ranges = append(ipamConf.Ranges, ranges...)
		}
		ipamConfig = ipamConf
	case "dhcp":
		ipamConf := newDHCPIPAMConfig()
		crd, err := defaults.CNIRuntimeDir()
		if err != nil {
			return nil, nil, err
		}
		ipamConf.DaemonSocketPath = filepath.Join(crd, "dhcp.sock")
		if err := systemutil.IsSocketAccessible(ipamConf.DaemonSocketPath); err != nil {
			log.L.Warnf("cannot access dhcp socket %q (hint: try running with `dhcp daemon --socketpath=%s &` in CNI_PATH to launch the dhcp daemon)", ipamConf.DaemonSocketPath, ipamConf.DaemonSocketPath)
		}

		// Set the host-name option to the value of passed argument NERDCTL_CNI_DHCP_HOSTNAME
		opts["host-name"] = `{"type": "provide", "fromArg": "NERDCTL_CNI_DHCP_HOSTNAME"}`

		// Convert all user-defined ipam-options into serializable options
		for optName, optValue := range opts {
			parsed := &struct {
				Type            string `json:"type"`
				Value           string `json:"value"`
				ValueFromCNIArg string `json:"fromArg"`
				SkipDefault     bool   `json:"skipDefault"`
			}{}
			if err := json.Unmarshal([]byte(optValue), parsed); err != nil {
				return nil, nil, fmt.Errorf("unparsable ipam option %s %q", optName, optValue)
			}
			if parsed.Type == "provide" {
				ipamConf.ProvideOptions = append(ipamConf.ProvideOptions, provideOption{
					Option:          optName,
					Value:           parsed.Value,
					ValueFromCNIArg: parsed.ValueFromCNIArg,
				})
			} else if parsed.Type == "request" {
				ipamConf.RequestOptions = append(ipamConf.RequestOptions, requestOption{
					Option:      optName,
					SkipDefault: parsed.SkipDefault,
				})
			} else {
				return nil, nil, fmt.Errorf("ipam option must have a type (provide or request)")
			}
		}

		ipamConfig = ipamConf
	default:
		return nil, nil, fmt.Errorf("unsupported ipam driver %q", driver)
	}

	ipam, err := structToMap(ipamConfig)
	if err != nil {
		return nil, nil, err
	}
	return ipam, auxBySubnet, nil
}

func (e *CNIEnv) parseIPAMRanges(subnets []string, gateways []string, ipRanges []string, aux map[string]string, ipv6 bool) ([][]IPAMRange, bool, map[string]map[string]string, error) {
	// Resolve every requested subnet first; parseSubnet also rejects overlaps
	// with existing networks. The pairing below then works purely on the parsed
	// CIDRs, so it can be unit-tested without probing the host's networks.
	parsedSubnets := make([]*net.IPNet, len(subnets))
	for i := range subnets {
		subnet, err := e.parseSubnet(subnets[i])
		if err != nil {
			return nil, false, nil, err
		}
		parsedSubnets[i] = subnet
	}
	return pairIPAMRanges(parsedSubnets, gateways, ipRanges, aux, ipv6)
}

// pairIPAMRanges matches each gateway, ip-range and aux-address to the subnet
// that contains it and builds the per-subnet IPAM ranges. It is split out from
// subnet resolution so the matching can be tested without touching live networks.
// The returned auxBySubnet maps each subnet CIDR to its reserved name=IP pairs so
// the caller can persist them outside the CNI config (host-local has no field for
// them); it is nil when no aux-address is given.
func pairIPAMRanges(subnets []*net.IPNet, gateways []string, ipRanges []string, aux map[string]string, ipv6 bool) ([][]IPAMRange, bool, map[string]map[string]string, error) {
	// Parse the gateways once up front; matching them to subnets below is then
	// just a containment check, with no parse error mixed into the loop.
	parsedGateways := make([]net.IP, len(gateways))
	for i, g := range gateways {
		gw := net.ParseIP(g)
		if gw == nil {
			return nil, false, nil, fmt.Errorf("failed to parse gateway %q", g)
		}
		parsedGateways[i] = gw
	}
	// Parse the ip-ranges the same way, keyed by network so each can be matched
	// to the subnet that contains it.
	parsedRanges := make([]*net.IPNet, len(ipRanges))
	for i, r := range ipRanges {
		_, ipNet, err := net.ParseCIDR(r)
		if err != nil {
			return nil, false, nil, fmt.Errorf("failed to parse ip-range %q", r)
		}
		parsedRanges[i] = ipNet
	}

	// Parse the aux-addresses once too, so the per-subnet loop only tests
	// containment. aux is already validated by ParseAuxAddresses, so every value
	// parses. matchedAux records which ones landed in a subnet, both to flag an
	// unmatched aux as an error and to attach each aux to only the first subnet
	// that contains it.
	parsedAux := make(map[string]net.IP, len(aux))
	for name, ipStr := range aux {
		parsedAux[name] = net.ParseIP(ipStr)
	}
	matchedAux := make(map[string]bool, len(parsedAux))

	findIPv4 := false
	ranges := make([][]IPAMRange, 0, len(subnets))
	// auxBySubnet holds each subnet's reserved name=IP pairs so the caller can
	// persist them in a nerdctl label; it stays nil unless an aux-address matches.
	var auxBySubnet map[string]map[string]string
	usedGateways := make([]bool, len(gateways))
	usedRanges := make([]bool, len(ipRanges))
	for _, subnet := range subnets {
		// if ipv6 flag is not set, subnets of ipv6 should be excluded
		if !ipv6 && subnet.IP.To4() == nil {
			continue
		}
		if !findIPv4 && subnet.IP.To4() != nil {
			findIPv4 = true
		}
		// Pair the subnet with the gateway it contains, so dual-stack matches
		// the v4 gateway to the v4 subnet and v6 to v6.
		gateway := ""
		for j, gw := range parsedGateways {
			if !usedGateways[j] && subnet.Contains(gw) {
				gateway, usedGateways[j] = gateways[j], true
				break
			}
		}
		// Pair the subnet with the ip-range it contains, the same way, so a
		// dual-stack network does not check the v4 range against the v6 subnet.
		ipRange := ""
		for j, r := range parsedRanges {
			if !usedRanges[j] && subnet.Contains(r.IP) {
				ipRange, usedRanges[j] = ipRanges[j], true
				break
			}
		}
		ipamRange, err := parseIPAMRange(subnet, gateway, ipRange)
		if err != nil {
			return nil, findIPv4, nil, err
		}
		// Collect the aux-addresses that fall inside this subnet, rejecting the
		// ones Docker also rejects (the network or gateway address), then reserve
		// them by splitting the range.
		gatewayIP := net.ParseIP(ipamRange.Gateway)
		subnetAux := map[string]string{}
		var reserved []net.IP
		for name, ip := range parsedAux {
			// Like gateway/ip-range, an aux-address attaches only to the first
			// subnet that contains it.
			if matchedAux[name] {
				continue
			}
			if !subnet.Contains(ip) {
				continue
			}
			matchedAux[name] = true
			if ip.Equal(subnet.IP) || (gatewayIP != nil && ip.Equal(gatewayIP)) {
				return nil, findIPv4, nil, fmt.Errorf("failed to allocate secondary ip address (%s:%s): Address already in use", name, ip)
			}
			subnetAux[name] = ip.String()
			reserved = append(reserved, ip)
		}
		rangeSet, err := splitIPAMRange(subnet, ipamRange, reserved)
		if err != nil {
			return nil, findIPv4, nil, err
		}
		// Record the reservation against the subnet CIDR host-local writes, so the
		// caller can store it in a label and inspect can match it back by subnet.
		if len(subnetAux) > 0 {
			if auxBySubnet == nil {
				auxBySubnet = map[string]map[string]string{}
			}
			auxBySubnet[ipamRange.Subnet] = subnetAux
		}
		ranges = append(ranges, rangeSet)
	}
	// Only known after every subnet is seen: a gateway, ip-range or aux-address
	// that matched no subnet is a user error, same as Docker.
	for j, ok := range usedGateways {
		if !ok {
			return nil, findIPv4, nil, fmt.Errorf("no matching subnet for gateway %q", gateways[j])
		}
	}
	for j, ok := range usedRanges {
		if !ok {
			return nil, findIPv4, nil, fmt.Errorf("no matching subnet for ip-range %q", ipRanges[j])
		}
	}
	// Report a stable IP: map iteration order is random, so sort the unmatched.
	var unmatchedAux []string
	for name, ip := range parsedAux {
		if !matchedAux[name] {
			unmatchedAux = append(unmatchedAux, ip.String())
		}
	}
	if len(unmatchedAux) > 0 {
		sort.Strings(unmatchedAux)
		return nil, findIPv4, nil, fmt.Errorf("no matching subnet for aux-address %s", unmatchedAux[0])
	}
	return ranges, findIPv4, auxBySubnet, nil
}

// FirewallPluginGEQVersion checks if the firewall plugin is greater than or equal to the specified version
func FirewallPluginGEQVersion(firewallPath string, versionStr string) (bool, error) {
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
	targetVer := semver.MustParse(versionStr)
	return ver.GreaterThan(targetVer) || ver.Equal(targetVer), nil
}

// guessFirewallPluginVersion guess the version of the CNI firewall plugin (not the version of the implemented CNI spec).
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
	return rootlessutil.WithDetachedNetNSIfAny(func() error {
		link, err := netlink.LinkByName(netIf)
		if err == nil {
			if err := netlink.LinkDel(link); err != nil {
				return fmt.Errorf("failed to remove network interface %s: %v", netIf, err)
			}
		}
		return nil
	})
}
