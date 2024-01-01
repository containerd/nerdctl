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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/mountutil"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/containerd/nerdctl/pkg/netutil/nettype"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/opencontainers/runtime-spec/specs-go"
)

const (
	UtsNamespaceHost = "host"
)

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

// NetworkOptionsManager types.NetworkOptionsManager is an interface for reading/setting networking
// options for containers based on the provided command flags.
type NetworkOptionsManager interface {
	// NetworkOptions Returns a copy of the internal types.NetworkOptions.
	NetworkOptions() types.NetworkOptions

	// VerifyNetworkOptions Verifies that the internal network settings are correct.
	VerifyNetworkOptions(context.Context) error

	// SetupNetworking Performs setup actions required for the container with the given ID.
	SetupNetworking(context.Context, string) error

	// CleanupNetworking Performs any required cleanup actions for the given container.
	// Should only be called to revert any setup steps performed in SetupNetworking.
	CleanupNetworking(context.Context, containerd.Container) error

	// InternalNetworkingOptionLabels Returns the set of NetworkingOptions which should be set as labels on the container.
	//
	// These options can potentially differ from the actual networking options
	// that the NetworkOptionsManager was initially instantiated with.
	// E.g: in container networking mode, the label will be normalized to an ID:
	// `--net=container:myContainer` => `--net=container:<ID of myContainer>`.
	InternalNetworkingOptionLabels(context.Context) (types.NetworkOptions, error)

	// ContainerNetworkingOpts Returns a slice of `oci.SpecOpts` and `containerd.NewContainerOpts` which represent
	// the network specs which need to be applied to the container with the given ID.
	ContainerNetworkingOpts(context.Context, string) ([]oci.SpecOpts, []containerd.NewContainerOpts, error)
}

// NewNetworkingOptionsManager Returns a types.NetworkOptionsManager based on the provided command's flags.
func NewNetworkingOptionsManager(globalOptions types.GlobalCommandOptions, netOpts types.NetworkOptions, client *containerd.Client) (NetworkOptionsManager, error) {
	netType, err := nettype.Detect(netOpts.NetworkSlice)
	if err != nil {
		return nil, err
	}

	var manager NetworkOptionsManager
	switch netType {
	case nettype.None:
		manager = &noneNetworkManager{globalOptions, netOpts, client}
	case nettype.Host:
		manager = &hostNetworkManager{globalOptions, netOpts, client}
	case nettype.Container:
		manager = &containerNetworkManager{globalOptions, netOpts, client}
	case nettype.CNI:
		manager = &cniNetworkManager{globalOptions, netOpts, client, cniNetworkManagerPlatform{}}
	default:
		return nil, fmt.Errorf("unexpected container networking type: %q", netType)
	}

	return manager, nil
}

// No-op types.NetworkOptionsManager for network-less containers.
type noneNetworkManager struct {
	globalOptions types.GlobalCommandOptions
	netOpts       types.NetworkOptions
	client        *containerd.Client
}

// NetworkOptions Returns a copy of the internal types.NetworkOptions.
func (m *noneNetworkManager) NetworkOptions() types.NetworkOptions {
	return m.netOpts
}

// VerifyNetworkOptions Verifies that the internal network settings are correct.
func (m *noneNetworkManager) VerifyNetworkOptions(_ context.Context) error {
	// No options to verify if no network settings are provided.
	return nil
}

// SetupNetworking Performs setup actions required for the container with the given ID.
func (m *noneNetworkManager) SetupNetworking(_ context.Context, _ string) error {
	return nil
}

// CleanupNetworking Performs any required cleanup actions for the given container.
// Should only be called to revert any setup steps performed in SetupNetworking.
func (m *noneNetworkManager) CleanupNetworking(_ context.Context, _ containerd.Container) error {
	return nil
}

// InternalNetworkingOptionLabels Returns the set of NetworkingOptions which should be set as labels on the container.
func (m *noneNetworkManager) InternalNetworkingOptionLabels(_ context.Context) (types.NetworkOptions, error) {
	return m.netOpts, nil
}

// ContainerNetworkingOpts Returns a slice of `oci.SpecOpts` and `containerd.NewContainerOpts` which represent
// the network specs which need to be applied to the container with the given ID.
func (m *noneNetworkManager) ContainerNetworkingOpts(_ context.Context, _ string) ([]oci.SpecOpts, []containerd.NewContainerOpts, error) {
	// No options to return if no network settings are provided.
	return []oci.SpecOpts{}, []containerd.NewContainerOpts{}, nil
}

// types.NetworkOptionsManager implementation for container networking settings.
type containerNetworkManager struct {
	globalOptions types.GlobalCommandOptions
	netOpts       types.NetworkOptions
	client        *containerd.Client
}

// NetworkOptions Returns a copy of the internal types.NetworkOptions.
func (m *containerNetworkManager) NetworkOptions() types.NetworkOptions {
	return m.netOpts
}

// VerifyNetworkOptions Verifies that the internal network settings are correct.
func (m *containerNetworkManager) VerifyNetworkOptions(_ context.Context) error {
	// TODO: check host OS, not client-side OS.
	if runtime.GOOS != "linux" {
		return errors.New("container networking mode is currently only supported on Linux")
	}

	if m.netOpts.NetworkSlice != nil && len(m.netOpts.NetworkSlice) > 1 {
		return errors.New("conflicting options: only one network specification is allowed when using '--network=container:<container>'")
	}

	nonZeroParams := nonZeroMapValues(map[string]interface{}{
		"--hostname":    m.netOpts.Hostname,
		"--mac-address": m.netOpts.MACAddress,
		// NOTE: an empty slice still counts as a non-zero value so we check its length:
		"-p/--publish": len(m.netOpts.PortMappings) != 0,
		"--dns":        len(m.netOpts.DNSServers) != 0,
		"--add-host":   len(m.netOpts.AddHost) != 0,
	})

	if len(nonZeroParams) != 0 {
		return fmt.Errorf("conflicting options: the following arguments are not supported when using `--network=container:<container>`: %s", nonZeroParams)
	}

	return nil
}

// Returns the relevant paths of the `hostname`, `resolv.conf`, and `hosts` files
// in the datastore of the container with the given ID.
func (m *containerNetworkManager) getContainerNetworkFilePaths(containerID string) (string, string, string, error) {
	dataStore, err := clientutil.DataStore(m.globalOptions.DataRoot, m.globalOptions.Address)
	if err != nil {
		return "", "", "", err
	}
	conStateDir, err := ContainerStateDirPath(m.globalOptions.Namespace, dataStore, containerID)
	if err != nil {
		return "", "", "", err
	}

	hostnamePath := filepath.Join(conStateDir, "hostname")
	resolvConfPath := filepath.Join(conStateDir, "resolv.conf")
	etcHostsPath := hostsstore.HostsPath(dataStore, m.globalOptions.Namespace, containerID)

	return hostnamePath, resolvConfPath, etcHostsPath, nil
}

// SetupNetworking Performs setup actions required for the container with the given ID.
func (m *containerNetworkManager) SetupNetworking(_ context.Context, _ string) error {
	// NOTE: container networking simply reuses network config files from the
	// bridged container so there are no setup/teardown steps required.
	return nil
}

// CleanupNetworking Performs any required cleanup actions for the given container.
// Should only be called to revert any setup steps performed in SetupNetworking.
func (m *containerNetworkManager) CleanupNetworking(_ context.Context, _ containerd.Container) error {
	// NOTE: container networking simply reuses network config files from the
	// bridged container so there are no setup/teardown steps required.
	return nil
}

// Searches for and returns the networking container for the given network argument.
func (m *containerNetworkManager) getNetworkingContainerForArgument(ctx context.Context, containerNetArg string, client *containerd.Client) (containerd.Container, error) {
	netItems := strings.Split(containerNetArg, ":")
	if len(netItems) < 2 {
		return nil, fmt.Errorf("container networking argument format must be 'container:<id|name>', got: %q", containerNetArg)
	}
	containerName := netItems[1]

	var foundContainer containerd.Container
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("container networking: multiple containers found with prefix: %s", containerName)
			}
			foundContainer = found.Container
			return nil
		},
	}
	n, err := walker.Walk(ctx, containerName)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, fmt.Errorf("container networking: could not find container: %s", containerName)
	}

	return foundContainer, nil
}

// InternalNetworkingOptionLabels Returns the set of NetworkingOptions which should be set as labels on the container.
func (m *containerNetworkManager) InternalNetworkingOptionLabels(ctx context.Context) (types.NetworkOptions, error) {
	opts := m.netOpts
	if m.netOpts.NetworkSlice == nil || len(m.netOpts.NetworkSlice) != 1 {
		return opts, fmt.Errorf("conflicting options: exactly one network specification is allowed when using '--network=container:<container>'")
	}

	container, err := m.getNetworkingContainerForArgument(ctx, m.netOpts.NetworkSlice[0], m.client)
	if err != nil {
		return opts, err
	}
	containerID := container.ID()
	opts.NetworkSlice = []string{fmt.Sprintf("container:%s", containerID)}
	return opts, nil
}

// ContainerNetworkingOpts Returns a slice of `oci.SpecOpts` and `containerd.NewContainerOpts` which represent
// the network specs which need to be applied to the container with the given ID.
func (m *containerNetworkManager) ContainerNetworkingOpts(ctx context.Context, _ string) ([]oci.SpecOpts, []containerd.NewContainerOpts, error) {
	opts := []oci.SpecOpts{}
	cOpts := []containerd.NewContainerOpts{}

	container, err := m.getNetworkingContainerForArgument(ctx, m.netOpts.NetworkSlice[0], m.client)
	if err != nil {
		return nil, nil, err
	}
	containerID := container.ID()

	s, err := container.Spec(ctx)
	if err != nil {
		return nil, nil, err
	}
	hostname := s.Hostname

	netNSPath, err := ContainerNetNSPath(ctx, container)
	if err != nil {
		return nil, nil, err
	}

	hostnamePath, resolvConfPath, etcHostsPath, err := m.getContainerNetworkFilePaths(containerID)
	if err != nil {
		return nil, nil, err
	}

	opts = append(opts,
		oci.WithLinuxNamespace(specs.LinuxNamespace{
			Type: specs.NetworkNamespace,
			Path: netNSPath,
		}),
		withCustomResolvConf(resolvConfPath),
		withCustomHosts(etcHostsPath),
		oci.WithHostname(hostname),
		withCustomEtcHostname(hostnamePath),
	)

	return opts, cOpts, nil
}

// types.NetworkOptionsManager implementation for host networking settings.
type hostNetworkManager struct {
	globalOptions types.GlobalCommandOptions
	netOpts       types.NetworkOptions
	client        *containerd.Client
}

// NetworkOptions Returns a copy of the internal types.NetworkOptions.
func (m *hostNetworkManager) NetworkOptions() types.NetworkOptions {
	return m.netOpts
}

// VerifyNetworkOptions Verifies that the internal network settings are correct.
func (m *hostNetworkManager) VerifyNetworkOptions(_ context.Context) error {
	// TODO: check host OS, not client-side OS.
	if runtime.GOOS == "windows" {
		return errors.New("cannot use host networking on Windows")
	}

	if m.netOpts.MACAddress != "" {
		return errors.New("conflicting options: mac-address and the network mode")
	}

	return validateUtsSettings(m.netOpts)
}

// SetupNetworking Performs setup actions required for the container with the given ID.
func (m *hostNetworkManager) SetupNetworking(_ context.Context, _ string) error {
	// NOTE: there are no setup steps required for host networking.
	return nil
}

// CleanupNetworking Performs any required cleanup actions for the given container.
// Should only be called to revert any setup steps performed in SetupNetworking.
func (m *hostNetworkManager) CleanupNetworking(_ context.Context, _ containerd.Container) error {
	// NOTE: there are no setup steps required for host networking.
	return nil
}

// InternalNetworkingOptionLabels Returns the set of NetworkingOptions which should be set as labels on the container.
func (m *hostNetworkManager) InternalNetworkingOptionLabels(_ context.Context) (types.NetworkOptions, error) {
	opts := m.netOpts
	// Cannot have a MAC address in host networking mode.
	opts.MACAddress = ""
	return opts, nil
}

// withDedupMounts Returns the specOpts if the mountPath is not in existing mounts.
// for https://github.com/containerd/nerdctl/issues/2685
func withDedupMounts(mountPath string, defaultSpec oci.SpecOpts) oci.SpecOpts {
	return func(ctx context.Context, client oci.Client, c *containers.Container, s *oci.Spec) error {
		for _, m := range s.Mounts {
			if m.Destination == mountPath {
				return nil
			}
		}
		return defaultSpec(ctx, client, c, s)
	}
}

func copyFileContent(src string, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	err = os.WriteFile(dst, data, 0644)
	if err != nil {
		return err
	}
	return nil
}

// ContainerNetworkingOpts Returns a slice of `oci.SpecOpts` and `containerd.NewContainerOpts` which represent
// the network specs which need to be applied to the container with the given ID.
func (m *hostNetworkManager) ContainerNetworkingOpts(_ context.Context, containerID string) ([]oci.SpecOpts, []containerd.NewContainerOpts, error) {

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
	copyFileContent("/etc/resolv.conf", resolvConfPath)

	etcHostsPath, err := hostsstore.AllocHostsFile(dataStore, m.globalOptions.Namespace, containerID)
	if err != nil {
		return nil, nil, err
	}
	copyFileContent("/etc/hosts", etcHostsPath)

	specs := []oci.SpecOpts{
		oci.WithHostNamespace(specs.NetworkNamespace),
		withDedupMounts("/etc/hosts", withCustomHosts(etcHostsPath)),
		withDedupMounts("/etc/resolv.conf", withCustomResolvConf(resolvConfPath)),
	}

	// `/etc/hostname` does not exist on FreeBSD
	if runtime.GOOS == "linux" && m.netOpts.UTSNamespace != UtsNamespaceHost {
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
			specs = append(specs, hostnameOpts...)
		}
	}

	return specs, cOpts, nil
}

// types.NetworkOptionsManager implementation for CNI networking settings.
// This is a more specialized and OS-dependendant networking model so this
// struct provides different implementations on different platforms.
type cniNetworkManager struct {
	globalOptions types.GlobalCommandOptions
	netOpts       types.NetworkOptions
	client        *containerd.Client
	cniNetworkManagerPlatform
}

// NetworkOptions Returns a copy of the internal types.NetworkOptions.
func (m *cniNetworkManager) NetworkOptions() types.NetworkOptions {
	return m.netOpts
}

func validateUtsSettings(netOpts types.NetworkOptions) error {
	utsNamespace := netOpts.UTSNamespace
	if utsNamespace == "" {
		return nil
	}

	// Docker considers this a validation error so keep compat.
	// https://docs.docker.com/engine/reference/run/#uts-settings---uts
	if utsNamespace == UtsNamespaceHost && netOpts.Hostname != "" {
		return fmt.Errorf("conflicting options: cannot set a --hostname with --uts=host")
	}

	return nil
}

// Writes the provided hostname string in a "hostname" file in the Container's
// Nerdctl-managed datastore and returns the oci.SpecOpts required in the container
// spec for the file to be mounted under /etc/hostname in the new container.
// If the hostname is empty, the leading 12 characters of the containerID
func writeEtcHostnameForContainer(globalOptions types.GlobalCommandOptions, hostname string, containerID string) ([]oci.SpecOpts, error) {
	if containerID == "" {
		return nil, fmt.Errorf("container ID is required for setting up hostname file")
	}

	dataStore, err := clientutil.DataStore(globalOptions.DataRoot, globalOptions.Address)
	if err != nil {
		return nil, err
	}

	stateDir, err := ContainerStateDirPath(globalOptions.Namespace, dataStore, containerID)
	if err != nil {
		return nil, err
	}

	hostnamePath := filepath.Join(stateDir, "hostname")
	if err := os.WriteFile(hostnamePath, []byte(hostname+"\n"), 0644); err != nil {
		return nil, err
	}

	return []oci.SpecOpts{oci.WithHostname(hostname), withCustomEtcHostname(hostnamePath)}, nil
}

// Loads all available networks and verifies that every selected network
// from the networkSlice is of a type within supportedTypes.
func verifyNetworkTypes(env *netutil.CNIEnv, networkSlice []string, supportedTypes []string) (map[string]*netutil.NetworkConfig, error) {
	netMap, err := env.NetworkMap()
	if err != nil {
		return nil, err
	}

	res := make(map[string]*netutil.NetworkConfig, len(networkSlice))
	for _, netstr := range networkSlice {
		netConfig, ok := netMap[netstr]
		if !ok {
			return nil, fmt.Errorf("network %s not found", netstr)
		}
		netType := netConfig.Plugins[0].Network.Type
		if supportedTypes != nil && !strutil.InStringSlice(supportedTypes, netType) {
			return nil, fmt.Errorf("network type %q is not supported for network mapping %q, must be one of: %v", netType, netstr, supportedTypes)
		}

		res[netstr] = netConfig
	}

	return res, nil
}

// NetworkOptionsFromSpec Returns the NetworkOptions used in a container's creation from its spec.Annotations.
func NetworkOptionsFromSpec(spec *specs.Spec) (types.NetworkOptions, error) {
	opts := types.NetworkOptions{}

	if spec == nil {
		return opts, fmt.Errorf("cannot determine networking options from nil spec")
	}
	if spec.Annotations == nil {
		return opts, fmt.Errorf("cannot determine networking options from nil spec.Annotations")
	}

	opts.Hostname = spec.Hostname

	if macAddress, ok := spec.Annotations[labels.MACAddress]; ok {
		opts.MACAddress = macAddress
	}

	if ipAddress, ok := spec.Annotations[labels.IPAddress]; ok {
		opts.IPAddress = ipAddress
	}

	var networks []string
	networksJSON := spec.Annotations[labels.Networks]
	if err := json.Unmarshal([]byte(networksJSON), &networks); err != nil {
		return opts, err
	}
	opts.NetworkSlice = networks

	if portsJSON := spec.Annotations[labels.Ports]; portsJSON != "" {
		if err := json.Unmarshal([]byte(portsJSON), &opts.PortMappings); err != nil {
			return opts, err
		}
	}

	return opts, nil
}

// Returns a lslice of keys of the values in the map that are invalid or are a non-zero-value
// for their respective type. (e.g. anything other than a `""` for string type)
// Note that the zero-values for innately pointer-types slices/maps/chans are `nil`,
// and NOT a zero-length container value like `[]Any{}`.
func nonZeroMapValues(values map[string]interface{}) []string {
	nonZero := []string{}

	for k, v := range values {
		if !reflect.ValueOf(v).IsZero() {
			nonZero = append(nonZero, k)
		}
	}

	return nonZero
}
