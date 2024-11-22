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
	"fmt"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/netns"
	"github.com/containerd/containerd/v2/pkg/oci"
	gocni "github.com/containerd/go-cni"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/netutil"
	"github.com/containerd/nerdctl/v2/pkg/ocihook"
)

type cniNetworkManagerPlatform struct {
	netNs *netns.NetNS
}

// Verifies that the internal network settings are correct.
func (m *cniNetworkManager) VerifyNetworkOptions(_ context.Context) error {
	e, err := netutil.NewCNIEnv(m.globalOptions.CNIPath, m.globalOptions.CNINetConfPath, netutil.WithNamespace(m.globalOptions.Namespace), netutil.WithDefaultNetwork(m.globalOptions.BridgeIP))
	if err != nil {
		return err
	}

	// NOTE: only currently supported network type on Windows is nat:
	validNetworkTypes := []string{"nat"}
	if _, err := verifyNetworkTypes(e, m.netOpts.NetworkSlice, validNetworkTypes); err != nil {
		return err
	}

	nonZeroArgs := nonZeroMapValues(map[string]interface{}{
		"--hostname": m.netOpts.Hostname,
		"--uts":      m.netOpts.UTSNamespace,
		// NOTE: IP setting is currently ignored on Windows.
		"--ip-address": m.netOpts.IPAddress,
		// NOTE: zero-length slices count as a non-zero-value so we explicitly check length:
		"--dns-opt/--dns-option": len(m.netOpts.DNSResolvConfOptions) != 0,
		"--dns-servers":          len(m.netOpts.DNSServers) != 0,
		"--dns-search":           len(m.netOpts.DNSSearchDomains) != 0,
		"--add-host":             len(m.netOpts.AddHost) != 0,
	})
	if len(nonZeroArgs) != 0 {
		return fmt.Errorf("the following networking arguments are not supported on Windows: %+v", nonZeroArgs)
	}

	return nil
}

func (m *cniNetworkManager) getCNI() (gocni.CNI, error) {
	e, err := netutil.NewCNIEnv(m.globalOptions.CNIPath, m.globalOptions.CNINetConfPath, netutil.WithNamespace(m.globalOptions.Namespace), netutil.WithDefaultNetwork(m.globalOptions.BridgeIP))
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate CNI env: %s", err)
	}

	cniOpts := []gocni.Opt{
		gocni.WithPluginDir([]string{m.globalOptions.CNIPath}),
		gocni.WithPluginConfDir(m.globalOptions.CNINetConfPath),
	}

	if netMap, err := verifyNetworkTypes(e, m.netOpts.NetworkSlice, nil); err == nil {
		for _, netConf := range netMap {
			cniOpts = append(cniOpts, gocni.WithConfListBytes(netConf.Bytes))
		}
	} else {
		return nil, err
	}

	return gocni.New(cniOpts...)
}

// Performs setup actions required for the container with the given ID.
func (m *cniNetworkManager) SetupNetworking(ctx context.Context, containerID string) error {
	cni, err := m.getCNI()
	if err != nil {
		return fmt.Errorf("failed to get container networking for setup: %s", err)
	}

	netNs, err := m.setupNetNs()
	if err != nil {
		return err
	}

	_, err = cni.Setup(ctx, containerID, netNs.GetPath(), m.getCNINamespaceOpts()...)
	return err
}

// Performs any required cleanup actions for the given container.
// Should only be called to revert any setup steps performed in setupNetworking.
func (m *cniNetworkManager) CleanupNetworking(ctx context.Context, container containerd.Container) error {
	containerID := container.ID()
	cni, err := m.getCNI()
	if err != nil {
		return fmt.Errorf("failed to get container networking for cleanup: %s", err)
	}

	spec, err := container.Spec(ctx)
	if err != nil {
		return fmt.Errorf("failed to get container specs for networking cleanup: %s", err)
	}

	netNsID, found := spec.Annotations[ocihook.NetworkNamespace]
	if !found {
		return fmt.Errorf("no %q annotation present on container with ID %s", ocihook.NetworkNamespace, containerID)
	}

	return cni.Remove(ctx, containerID, netNsID, m.getCNINamespaceOpts()...)
}

// Returns the set of NetworkingOptions which should be set as labels on the container.
func (m *cniNetworkManager) InternalNetworkingOptionLabels(_ context.Context) (types.NetworkOptions, error) {
	return m.netOpts, nil
}

// Returns a slice of `oci.SpecOpts` and `containerd.NewContainerOpts` which represent
// the network specs which need to be applied to the container with the given ID.
func (m *cniNetworkManager) ContainerNetworkingOpts(_ context.Context, containerID string) ([]oci.SpecOpts, []containerd.NewContainerOpts, error) {
	ns, err := m.setupNetNs()
	if err != nil {
		return nil, nil, err
	}

	opts := []oci.SpecOpts{
		oci.WithWindowNetworksAllowUnqualifiedDNSQuery(),
		oci.WithWindowsNetworkNamespace(ns.GetPath()),
	}

	cOpts := []containerd.NewContainerOpts{
		containerd.WithAdditionalContainerLabels(
			map[string]string{
				ocihook.NetworkNamespace: ns.GetPath(),
			},
		),
	}

	return opts, cOpts, nil
}

// Returns the string path to a network namespace.
func (m *cniNetworkManager) setupNetNs() (*netns.NetNS, error) {
	if m.netNs != nil {
		return m.netNs, nil
	}

	// NOTE: the baseDir argument to NewNetNS is ignored on Windows.
	ns, err := netns.NewNetNS("")
	if err != nil {
		return nil, err
	}

	m.netNs = ns
	return ns, err
}

// Returns the []gocni.NamespaceOpts to be used for CNI setup/teardown.
func (m *cniNetworkManager) getCNINamespaceOpts() []gocni.NamespaceOpts {
	opts := []gocni.NamespaceOpts{
		gocni.WithLabels(map[string]string{
			// allow loose CNI argument verification
			// FYI: https://github.com/containernetworking/cni/issues/560
			"IgnoreUnknown": "1",
		}),
	}

	if m.netOpts.MACAddress != "" {
		opts = append(opts, gocni.WithArgs("MAC", m.netOpts.MACAddress))
	}

	if m.netOpts.IPAddress != "" {
		opts = append(opts, gocni.WithArgs("IP", m.netOpts.IPAddress))
	}

	if m.netOpts.PortMappings != nil {
		opts = append(opts, gocni.WithCapabilityPortMap(m.netOpts.PortMappings))
	}

	return opts
}
