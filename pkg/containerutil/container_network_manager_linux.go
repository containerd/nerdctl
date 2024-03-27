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

package containerutil

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/dnsutil"
	"github.com/containerd/nerdctl/v2/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/v2/pkg/netutil"
	"github.com/containerd/nerdctl/v2/pkg/resolvconf"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
)

type cniNetworkManagerPlatform struct {
}

// Verifies that the internal network settings are correct.
func (m *cniNetworkManager) VerifyNetworkOptions(_ context.Context) error {
	e, err := netutil.NewCNIEnv(m.globalOptions.CNIPath, m.globalOptions.CNINetConfPath, netutil.WithDefaultNetwork())
	if err != nil {
		return err
	}

	if m.netOpts.MACAddress != "" {
		macValidNetworks := []string{"bridge", "macvlan"}
		if _, err := verifyNetworkTypes(e, m.netOpts.NetworkSlice, macValidNetworks); err != nil {
			return err
		}
	}

	return validateUtsSettings(m.netOpts)
}

// Performs setup actions required for the container with the given ID.
func (m *cniNetworkManager) SetupNetworking(_ context.Context, _ string) error {
	// NOTE: on non-Windows systems which support OCI hooks, CNI networking setup
	// is performed via createRuntime and postCreate hooks whose logic can
	// be found in the pkg/ocihook package.
	return nil
}

// Performs any required cleanup actions for the given container.
// Should only be called to revert any setup steps performed in setupNetworking.
func (m *cniNetworkManager) CleanupNetworking(_ context.Context, _ containerd.Container) error {
	// NOTE: on non-Windows systems which support OCI hooks, CNI networking setup
	// is performed via createRuntime and postCreate hooks whose logic can
	// be found in the pkg/ocihook package.
	return nil
}

// Returns the set of NetworkingOptions which should be set as labels on the container.
func (m *cniNetworkManager) InternalNetworkingOptionLabels(_ context.Context) (types.NetworkOptions, error) {
	return m.netOpts, nil
}

// Returns a slice of `oci.SpecOpts` and `containerd.NewContainerOpts` which represent
// the network specs which need to be applied to the container with the given ID.
func (m *cniNetworkManager) ContainerNetworkingOpts(_ context.Context, containerID string) ([]oci.SpecOpts, []containerd.NewContainerOpts, error) {
	opts := []oci.SpecOpts{}
	cOpts := []containerd.NewContainerOpts{}

	dataStore, err := clientutil.DataStore(m.globalOptions.DataRoot, m.globalOptions.Address)
	if err != nil {
		return nil, nil, err
	}

	stateDir, err := ContainerStateDirPath(m.globalOptions.Namespace, dataStore, containerID)
	if err != nil {
		return nil, nil, err
	}

	resolvConfPath := filepath.Join(stateDir, "resolv.conf")
	if err := m.buildResolvConf(resolvConfPath); err != nil {
		return nil, nil, err
	}

	// the content of /etc/hosts is created in OCI Hook
	etcHostsPath, err := hostsstore.AllocHostsFile(dataStore, m.globalOptions.Namespace, containerID)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, withCustomResolvConf(resolvConfPath), withCustomHosts(etcHostsPath))

	if m.netOpts.UTSNamespace != UtsNamespaceHost {
		// If no hostname is set, default to first 12 characters of the container ID.
		hostname := m.netOpts.Hostname
		if hostname == "" {
			hostname = containerID
			if len(hostname) > 12 {
				hostname = hostname[0:12]
			}
		}
		m.netOpts.Hostname = hostname

		hostnameOpts, err := writeEtcHostnameForContainer(m.globalOptions, m.netOpts.Hostname, containerID)
		if err != nil {
			return nil, nil, err
		}
		if hostnameOpts != nil {
			opts = append(opts, hostnameOpts...)
		}
	}

	return opts, cOpts, nil
}

func (m *cniNetworkManager) buildResolvConf(resolvConfPath string) error {
	var err error
	slirp4Dns := []string{}
	if rootlessutil.IsRootlessChild() {
		slirp4Dns, err = dnsutil.GetSlirp4netnsDNS()
		if err != nil {
			return err
		}
	}

	var (
		nameServers   = m.netOpts.DNSServers
		searchDomains = m.netOpts.DNSSearchDomains
		dnsOptions    = m.netOpts.DNSResolvConfOptions
	)

	// Use host defaults if any DNS settings are missing:
	if len(nameServers) == 0 || len(searchDomains) == 0 || len(dnsOptions) == 0 {
		conf, err := resolvconf.Get()
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			// if resolvConf file does't exist, using default resolvers
			conf = &resolvconf.File{}
			log.L.WithError(err).Debugf("resolvConf file doesn't exist on host")
		}
		conf, err = resolvconf.FilterResolvDNS(conf.Content, true)
		if err != nil {
			return err
		}
		if len(nameServers) == 0 {
			nameServers = resolvconf.GetNameservers(conf.Content, resolvconf.IPv4)
		}
		if len(searchDomains) == 0 {
			searchDomains = resolvconf.GetSearchDomains(conf.Content)
		}
		if len(dnsOptions) == 0 {
			dnsOptions = resolvconf.GetOptions(conf.Content)
		}
	}

	_, err = resolvconf.Build(resolvConfPath, append(slirp4Dns, nameServers...), searchDomains, dnsOptions)
	return err
}
