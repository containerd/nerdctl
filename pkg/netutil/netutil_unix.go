//go:build freebsd || linux
// +build freebsd linux

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
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/containerd/nerdctl/pkg/systemutil"
	"github.com/sirupsen/logrus"
)

const (
	DefaultNetworkName = "bridge"
	DefaultID          = 0
	DefaultCIDR        = "10.4.0.0/24"
	DefaultIPAMDriver  = "host-local"
)

func (e *CNIEnv) GenerateCNIPlugins(driver string, id int, name string, ipam map[string]interface{}, opts map[string]string) ([]CNIPlugin, error) {
	var (
		plugins []CNIPlugin
		err     error
	)
	switch driver {
	case "bridge":
		mtu := 0
		for opt, v := range opts {
			switch opt {
			case "mtu", "com.docker.network.driver.mtu":
				mtu, err = ParseMTU(v)
				if err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("unsupported %q network option %q", driver, opt)
			}
		}
		bridge := newBridgePlugin(GetBridgeName(id))
		bridge.MTU = mtu
		bridge.IPAM = ipam
		bridge.IsGW = true
		bridge.IPMasq = true
		bridge.HairpinMode = true
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
		plugins = []CNIPlugin{vlan}
	default:
		return nil, fmt.Errorf("unsupported cni driver %q", driver)
	}
	return plugins, nil
}

func GenerateIPAM(driver string, subnetStr, gatewayStr, ipRangeStr string, opts map[string]string) (map[string]interface{}, error) {
	var ipamConfig interface{}
	switch driver {
	case "default", "host-local":
		ipamRange, err := parseIPAMRange(subnetStr, gatewayStr, ipRangeStr)
		if err != nil {
			return nil, err
		}

		ipamConf := newHostLocalIPAMConfig()
		ipamConf.Routes = []IPAMRoute{
			{Dst: "0.0.0.0/0"},
		}
		ipamConf.Ranges = append(ipamConf.Ranges, []IPAMRange{*ipamRange})
		ipamConfig = ipamConf
	case "dhcp":
		ipamConf := newDHCPIPAMConfig()
		ipamConf.DaemonSocketPath = filepath.Join(defaults.CNIRuntimeDir(), "dhcp.sock")
		// TODO: support IPAM options for dhcp
		if err := systemutil.IsSocketAccessible(ipamConf.DaemonSocketPath); err != nil {
			logrus.Warnf("cannot access dhcp socket %q (hint: try running with `dhcp daemon --socketpath=%s &` in CNI_PATH to launch the dhcp daemon)", ipamConf.DaemonSocketPath, ipamConf.DaemonSocketPath)
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

func fixUpIsolation(e *CNIEnv, name string, plugins []CNIPlugin) []CNIPlugin {
	isolationPath := filepath.Join(e.Path, "isolation")
	if _, err := exec.LookPath(isolationPath); err == nil {
		// the warning is suppressed for DefaultNetworkName (because multi-bridge networking is not involved)
		if name != DefaultNetworkName {
			logrus.Warnf(`network %q: Using the deprecated CNI "isolation" plugin instead of CNI "firewall" plugin (>= 1.1.0) ingressPolicy.
To dismiss this warning, uninstall %q and install CNI "firewall" plugin (>= 1.1.0) from https://github.com/containernetworking/plugins`,
				name, isolationPath)
		}
		plugins = append(plugins, newIsolationPlugin())
		for _, f := range plugins {
			if x, ok := f.(*firewallConfig); ok {
				if name != DefaultNetworkName {
					logrus.Warnf("network %q: Unsetting firewall ingressPolicy %q (because using the deprecated \"isolation\" plugin)", name, x.IngressPolicy)
				}
				x.IngressPolicy = ""
			}
		}
	} else if name != DefaultNetworkName {
		firewallPath := filepath.Join(e.Path, "firewall")
		ok, err := firewallPluginGEQ110(firewallPath)
		if err != nil {
			logrus.WithError(err).Warnf("Failed to detect whether %q is newer than v1.1.0", firewallPath)
		}
		if !ok {
			logrus.Warnf("To isolate bridge networks, CNI plugin \"firewall\" (>= 1.1.0), or CNI plugin \"isolation\" (deprecated) needs to be installed in CNI_PATH (%q), see https://github.com/containernetworking/plugins",
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
