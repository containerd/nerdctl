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

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/pkg/dnsutil"
	"github.com/containerd/nerdctl/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/pkg/mountutil"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/containerd/nerdctl/pkg/netutil/nettype"
	"github.com/containerd/nerdctl/pkg/portutil"
	"github.com/containerd/nerdctl/pkg/resolvconf"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/spf13/cobra"
)

func getNetworkSlice(cmd *cobra.Command) ([]string, error) {
	var netSlice = []string{}
	var networkSet = false
	if cmd.Flags().Lookup("network").Changed {
		network, err := cmd.Flags().GetStringSlice("network")
		if err != nil {
			return nil, err
		}
		netSlice = append(netSlice, network...)
		networkSet = true
	}
	if cmd.Flags().Lookup("net").Changed {
		net, err := cmd.Flags().GetStringSlice("net")
		if err != nil {
			return nil, err
		}
		netSlice = append(netSlice, net...)
		networkSet = true
	}

	if !networkSet {
		network, err := cmd.Flags().GetStringSlice("network")
		if err != nil {
			return nil, err
		}
		netSlice = append(netSlice, network...)
	}
	return netSlice, nil
}

func withCustomResolvConf(src string) func(context.Context, oci.Client, *containers.Container, *oci.Spec) error {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		s.Mounts = append(s.Mounts, specs.Mount{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      src,
			Options:     []string{"bind", mountutil.DefaultPropagationMode}, // writable
		})
		return nil
	}
}

func withCustomEtcHostname(src string) func(context.Context, oci.Client, *containers.Container, *oci.Spec) error {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		s.Mounts = append(s.Mounts, specs.Mount{
			Destination: "/etc/hostname",
			Type:        "bind",
			Source:      src,
			Options:     []string{"bind", mountutil.DefaultPropagationMode}, // writable
		})
		return nil
	}
}

func withCustomHosts(src string) func(context.Context, oci.Client, *containers.Container, *oci.Spec) error {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		s.Mounts = append(s.Mounts, specs.Mount{
			Destination: "/etc/hosts",
			Type:        "bind",
			Source:      src,
			Options:     []string{"bind", mountutil.DefaultPropagationMode}, // writable
		})
		return nil
	}
}

func generateNetOpts(cmd *cobra.Command, dataStore, stateDir, ns, id string) ([]oci.SpecOpts, []string, []gocni.PortMapping, error) {
	opts := []oci.SpecOpts{}
	portSlice, err := cmd.Flags().GetStringSlice("publish")
	if err != nil {
		return nil, nil, nil, err
	}
	netSlice, err := getNetworkSlice(cmd)
	if err != nil {
		return nil, nil, nil, err
	}

	ports := make([]gocni.PortMapping, 0)
	netType, err := nettype.Detect(netSlice)
	if err != nil {
		return nil, nil, nil, err
	}

	switch netType {
	case nettype.None:
		// NOP
	case nettype.Host:
		opts = append(opts, oci.WithHostNamespace(specs.NetworkNamespace), oci.WithHostHostsFile, oci.WithHostResolvconf)
	case nettype.CNI:
		// We only verify flags and generate resolv.conf here.
		// The actual network is configured in the oci hook.
		cniPath, err := cmd.Flags().GetString("cni-path")
		if err != nil {
			return nil, nil, nil, err
		}
		cniNetconfpath, err := cmd.Flags().GetString("cni-netconfpath")
		if err != nil {
			return nil, nil, nil, err
		}
		e, err := netutil.NewCNIEnv(cniPath, cniNetconfpath)
		if err != nil {
			return nil, nil, nil, err
		}
		netMap := e.NetworkMap()
		for _, netstr := range netSlice {
			_, ok := netMap[netstr]
			if !ok {
				return nil, nil, nil, fmt.Errorf("network %s not found", netstr)
			}
		}

		resolvConfPath := filepath.Join(stateDir, "resolv.conf")
		dnsValue, err := cmd.Flags().GetStringSlice("dns")
		if err != nil {
			return nil, nil, nil, err
		}
		if runtime.GOOS == "linux" {
			conf, err := resolvconf.Get()
			if err != nil {
				return nil, nil, nil, err
			}
			slirp4Dns := []string{}
			if rootlessutil.IsRootlessChild() {
				slirp4Dns, err = dnsutil.GetSlirp4netnsDns()
				if err != nil {
					return nil, nil, nil, err
				}
			}
			conf, err = resolvconf.FilterResolvDNS(conf.Content, true)
			if err != nil {
				return nil, nil, nil, err
			}
			searchDomains := resolvconf.GetSearchDomains(conf.Content)
			dnsOptions := resolvconf.GetOptions(conf.Content)
			nameServers := strutil.DedupeStrSlice(dnsValue)
			if len(nameServers) == 0 {
				nameServers = resolvconf.GetNameservers(conf.Content, resolvconf.IPv4)
			}
			if _, err := resolvconf.Build(resolvConfPath, append(slirp4Dns, nameServers...), searchDomains, dnsOptions); err != nil {
				return nil, nil, nil, err
			}

			// the content of /etc/hosts is created in OCI Hook
			etcHostsPath, err := hostsstore.AllocHostsFile(dataStore, ns, id)
			if err != nil {
				return nil, nil, nil, err
			}
			opts = append(opts, withCustomResolvConf(resolvConfPath), withCustomHosts(etcHostsPath))
			for _, p := range portSlice {
				pm, err := portutil.ParseFlagP(p)
				if err != nil {
					return nil, nil, pm, err
				}
				ports = append(ports, pm...)
			}
		}
	default:
		return nil, nil, nil, fmt.Errorf("unexpected network type %v", netType)
	}
	return opts, netSlice, ports, nil
}
