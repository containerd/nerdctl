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

package ocihook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	types100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/opencontainers/runtime-spec/specs-go"
	b4nndclient "github.com/rootless-containers/bypass4netns/pkg/api/daemon/client"
	rlkclient "github.com/rootless-containers/rootlesskit/v2/pkg/api/client"

	"github.com/containerd/go-cni"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/bypass4netnsutil"
	"github.com/containerd/nerdctl/v2/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/lockutil"
	"github.com/containerd/nerdctl/v2/pkg/namestore"
	"github.com/containerd/nerdctl/v2/pkg/netutil"
	"github.com/containerd/nerdctl/v2/pkg/netutil/nettype"
	"github.com/containerd/nerdctl/v2/pkg/ocihook/state"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/store"
)

const (
	// NetworkNamespace is the network namespace path to be passed to the CNI plugins.
	// When this annotation is set from the runtime spec.State payload, it takes
	// precedence over the PID based resolution (/proc/<pid>/ns/net) where pid is
	// spec.State.Pid.
	// This is mostly used for VM based runtime, where the spec.State PID does not
	// necessarily lives in the created container networking namespace.
	//
	// On Windows, this label will contain the UUID of a namespace managed by
	// the Host Compute Network Service (HCN) API.
	NetworkNamespace = labels.Prefix + "network-namespace"
)

func Run(stdin io.Reader, stderr io.Writer, event, dataStore, cniPath, cniNetconfPath, bridgeIP string) error {
	if stdin == nil || event == "" || dataStore == "" || cniPath == "" || cniNetconfPath == "" {
		return errors.New("got insufficient args")
	}

	var state specs.State
	if err := json.NewDecoder(stdin).Decode(&state); err != nil {
		return err
	}

	containerStateDir := state.Annotations[labels.StateDir]
	if containerStateDir == "" {
		return errors.New("state dir must be set")
	}
	if err := os.MkdirAll(containerStateDir, 0700); err != nil {
		return fmt.Errorf("failed to create %q: %w", containerStateDir, err)
	}
	logFilePath := filepath.Join(containerStateDir, "oci-hook."+event+".log")
	logFile, err := os.Create(logFilePath)
	if err != nil {
		return err
	}
	currentOutput := log.L.Logger.Out
	log.L.Logger.SetOutput(io.MultiWriter(stderr, logFile))
	defer func() {
		log.L.Logger.SetOutput(currentOutput)
		err = logFile.Close()
		if err != nil {
			log.L.Logger.WithError(err).Error("failed closing oci hook log file")
		}
	}()

	// FIXME: CNI plugins are not safe to use concurrently
	// See
	// https://github.com/containerd/nerdctl/issues/3518
	// https://github.com/containerd/nerdctl/issues/2908
	// and likely others
	// Fixing these issues would require a lot of work, possibly even stopping using individual cni binaries altogether
	// or at least being very mindful in what operation we call inside CNIEnv at what point, with filesystem locking.
	// This below is a stopgap solution that just enforces a global lock
	// Note this here is probably not enough, as concurrent CNI operations may happen outside of the scope of ocihooks
	// through explicit calls to Remove, etc.
	// Finally note that this is not the same (albeit similar) as libcni filesystem manipulation locking,
	// hence the independent lock
	err = os.MkdirAll(cniNetconfPath, 0o700)
	if err != nil {
		return err
	}
	lock, err := lockutil.Lock(filepath.Join(cniNetconfPath, ".cni-concurrency.lock"))
	if err != nil {
		return err
	}
	defer lockutil.Unlock(lock)

	opts, err := newHandlerOpts(&state, dataStore, cniPath, cniNetconfPath, bridgeIP)
	if err != nil {
		return err
	}

	switch event {
	case "createRuntime":
		return onCreateRuntime(opts)
	case "postStop":
		return onPostStop(opts)
	default:
		return fmt.Errorf("unexpected event %q", event)
	}
}

func newHandlerOpts(state *specs.State, dataStore, cniPath, cniNetconfPath, bridgeIP string) (*handlerOpts, error) {
	o := &handlerOpts{
		state:     state,
		dataStore: dataStore,
	}

	extraHosts, err := getExtraHosts(state)
	if err != nil {
		return nil, err
	}
	o.extraHosts = extraHosts

	hs, err := loadSpec(o.state.Bundle)
	if err != nil {
		return nil, err
	}
	o.rootfs = hs.Root.Path
	if !filepath.IsAbs(o.rootfs) {
		o.rootfs = filepath.Join(o.state.Bundle, o.rootfs)
	}

	namespace := o.state.Annotations[labels.Namespace]
	if namespace == "" {
		return nil, errors.New("namespace must be set")
	}
	if o.state.ID == "" {
		return nil, errors.New("state.ID must be set")
	}
	o.fullID = namespace + "-" + o.state.ID

	networksJSON := o.state.Annotations[labels.Networks]
	var networks []string
	if err := json.Unmarshal([]byte(networksJSON), &networks); err != nil {
		return nil, err
	}

	netType, err := nettype.Detect(networks)
	if err != nil {
		return nil, err
	}

	switch netType {
	case nettype.Host, nettype.None, nettype.Container, nettype.Namespace:
		// NOP
	case nettype.CNI:
		e, err := netutil.NewCNIEnv(cniPath, cniNetconfPath, netutil.WithNamespace(namespace), netutil.WithDefaultNetwork(bridgeIP))
		if err != nil {
			return nil, err
		}
		cniOpts := []cni.Opt{
			cni.WithPluginDir([]string{cniPath}),
		}
		var netw *netutil.NetworkConfig
		for _, netstr := range networks {
			if netw, err = e.NetworkByNameOrID(netstr); err != nil {
				return nil, err
			}
			cniOpts = append(cniOpts, cni.WithConfListBytes(netw.Bytes))
			o.cniNames = append(o.cniNames, netstr)
		}
		o.cni, err = cni.New(cniOpts...)
		if err != nil {
			return nil, err
		}
		if o.cni == nil {
			log.L.Warnf("no CNI network could be loaded from the provided network names: %v", networks)
		}
	default:
		return nil, fmt.Errorf("unexpected network type %v", netType)
	}

	if pidFile := o.state.Annotations[labels.PIDFile]; pidFile != "" {
		if err := writePidFile(pidFile, state.Pid); err != nil {
			return nil, err
		}
	}

	if portsJSON := o.state.Annotations[labels.Ports]; portsJSON != "" {
		if err := json.Unmarshal([]byte(portsJSON), &o.ports); err != nil {
			return nil, err
		}
	}

	if ipAddress, ok := o.state.Annotations[labels.IPAddress]; ok {
		o.containerIP = ipAddress
	}

	if macAddress, ok := o.state.Annotations[labels.MACAddress]; ok {
		o.containerMAC = macAddress
	}

	if ip6Address, ok := o.state.Annotations[labels.IP6Address]; ok {
		o.containerIP6 = ip6Address
	}

	if rootlessutil.IsRootlessChild() {
		o.rootlessKitClient, err = rootlessutil.NewRootlessKitClient()
		if err != nil {
			return nil, err
		}
		b4nnEnabled, _, err := bypass4netnsutil.IsBypass4netnsEnabled(o.state.Annotations)
		if err != nil {
			return nil, err
		}
		if b4nnEnabled {
			socketPath, err := bypass4netnsutil.GetBypass4NetnsdDefaultSocketPath()
			if err != nil {
				return nil, err
			}
			o.bypassClient, err = b4nndclient.New(socketPath)
			if err != nil {
				return nil, fmt.Errorf("bypass4netnsd not running? (Hint: run `containerd-rootless-setuptool.sh install-bypass4netnsd`): %w", err)
			}
		}
	}
	return o, nil
}

type handlerOpts struct {
	state             *specs.State
	dataStore         string
	rootfs            string
	ports             []cni.PortMapping
	cni               cni.CNI
	cniNames          []string
	fullID            string
	rootlessKitClient rlkclient.Client
	bypassClient      b4nndclient.Client
	extraHosts        map[string]string // host:ip
	containerIP       string
	containerMAC      string
	containerIP6      string
}

// hookSpec is from https://github.com/containerd/containerd/blob/v1.4.3/cmd/containerd/command/oci-hook.go#L59-L64
type hookSpec struct {
	Root struct {
		Path string `json:"path"`
	} `json:"root"`
}

// loadSpec is from https://github.com/containerd/containerd/blob/v1.4.3/cmd/containerd/command/oci-hook.go#L65-L76
func loadSpec(bundle string) (*hookSpec, error) {
	f, err := os.Open(filepath.Join(bundle, "config.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var s hookSpec
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func getExtraHosts(state *specs.State) (map[string]string, error) {
	extraHostsJSON := state.Annotations[labels.ExtraHosts]
	var extraHosts []string
	if err := json.Unmarshal([]byte(extraHostsJSON), &extraHosts); err != nil {
		return nil, err
	}

	hosts := make(map[string]string)
	for _, host := range extraHosts {
		if v := strings.SplitN(host, ":", 2); len(v) == 2 {
			hosts[v[0]] = v[1]
		}
	}
	return hosts, nil
}

func getNetNSPath(state *specs.State) (string, error) {
	// If we have a network-namespace annotation we use it over the passed Pid.
	netNsPath, netNsFound := state.Annotations[NetworkNamespace]
	if netNsFound {
		if _, err := os.Stat(netNsPath); err != nil {
			return "", err
		}

		return netNsPath, nil
	}

	if state.Pid == 0 {
		return "", errors.New("both state.Pid and the netNs annotation are unset")
	}

	// We dont't have a networking namespace annotation, but we have a PID.
	s := fmt.Sprintf("/proc/%d/ns/net", state.Pid)
	if _, err := os.Stat(s); err != nil {
		return "", err
	}
	return s, nil
}

func getPortMapOpts(opts *handlerOpts) ([]cni.NamespaceOpts, error) {
	if len(opts.ports) > 0 {
		if !rootlessutil.IsRootlessChild() {
			return []cni.NamespaceOpts{cni.WithCapabilityPortMap(opts.ports)}, nil
		}
		var (
			childIP                            net.IP
			portDriverDisallowsLoopbackChildIP bool
		)
		info, err := opts.rootlessKitClient.Info(context.TODO())
		if err != nil {
			log.L.WithError(err).Warn("cannot call RootlessKit Info API, make sure you have RootlessKit v0.14.1 or later")
		} else {
			childIP = info.NetworkDriver.ChildIP
			portDriverDisallowsLoopbackChildIP = info.PortDriver.DisallowLoopbackChildIP // true for slirp4netns port driver
		}
		// For rootless, we need to modify the hostIP that is not bindable in the child namespace.
		// https: //github.com/containerd/nerdctl/issues/88
		//
		// We must NOT modify opts.ports here, because we use the unmodified opts.ports for
		// interaction with RootlessKit API.
		ports := make([]cni.PortMapping, len(opts.ports))
		for i, p := range opts.ports {
			if hostIP := net.ParseIP(p.HostIP); hostIP != nil && !hostIP.IsUnspecified() {
				// loopback address is always bindable in the child namespace, but other addresses are unlikely.
				if !hostIP.IsLoopback() {
					if !(childIP != nil && childIP.Equal(hostIP)) {
						if portDriverDisallowsLoopbackChildIP {
							p.HostIP = childIP.String()
						} else {
							p.HostIP = "127.0.0.1"
						}
					}
				} else if portDriverDisallowsLoopbackChildIP {
					p.HostIP = childIP.String()
				}
			}
			ports[i] = p
		}
		return []cni.NamespaceOpts{cni.WithCapabilityPortMap(ports)}, nil
	}
	return nil, nil
}

func getIPAddressOpts(opts *handlerOpts) ([]cni.NamespaceOpts, error) {
	if opts.containerIP != "" {
		if rootlessutil.IsRootlessChild() {
			log.L.Debug("container IP assignment is not fully supported in rootless mode. The IP is not accessible from the host (but still accessible from other containers).")
		}

		return []cni.NamespaceOpts{
			cni.WithLabels(map[string]string{
				// Special tick for go-cni. Because go-cni marks all labels and args as same
				// So, we need add a special label to pass the containerIP to the host-local plugin.
				// FYI: https://github.com/containerd/go-cni/blob/v1.1.3/README.md?plain=1#L57-L64
				"IgnoreUnknown": "1",
			}),
			cni.WithArgs("IP", opts.containerIP),
		}, nil
	}
	return nil, nil
}

func getMACAddressOpts(opts *handlerOpts) ([]cni.NamespaceOpts, error) {
	if opts.containerMAC != "" {
		return []cni.NamespaceOpts{
			cni.WithLabels(map[string]string{
				// allow loose CNI argument verification
				// FYI: https://github.com/containernetworking/cni/issues/560
				"IgnoreUnknown": "1",
			}),
			cni.WithArgs("MAC", opts.containerMAC),
		}, nil
	}
	return nil, nil
}

func getIP6AddressOpts(opts *handlerOpts) ([]cni.NamespaceOpts, error) {
	if opts.containerIP6 != "" {
		if rootlessutil.IsRootlessChild() {
			log.L.Debug("container IP6 assignment is not fully supported in rootless mode. The IP6 is not accessible from the host (but still accessible from other containers).")
		}
		return []cni.NamespaceOpts{
			cni.WithLabels(map[string]string{
				// allow loose CNI argument verification
				// FYI: https://github.com/containernetworking/cni/issues/560
				"IgnoreUnknown": "1",
			}),
			cni.WithCapability("ips", []string{opts.containerIP6}),
		}, nil
	}
	return nil, nil
}

func applyNetworkSettings(opts *handlerOpts) (err error) {
	portMapOpts, err := getPortMapOpts(opts)
	if err != nil {
		return err
	}
	nsPath, err := getNetNSPath(opts.state)
	if err != nil {
		return err
	}
	ctx := context.Background()
	hs, err := hostsstore.New(opts.dataStore, opts.state.Annotations[labels.Namespace])
	if err != nil {
		return err
	}
	ipAddressOpts, err := getIPAddressOpts(opts)
	if err != nil {
		return err
	}
	macAddressOpts, err := getMACAddressOpts(opts)
	if err != nil {
		return err
	}
	ip6AddressOpts, err := getIP6AddressOpts(opts)
	if err != nil {
		return err
	}
	var namespaceOpts []cni.NamespaceOpts
	namespaceOpts = append(namespaceOpts, portMapOpts...)
	namespaceOpts = append(namespaceOpts, ipAddressOpts...)
	namespaceOpts = append(namespaceOpts, macAddressOpts...)
	namespaceOpts = append(namespaceOpts, ip6AddressOpts...)
	namespaceOpts = append(namespaceOpts,
		cni.WithLabels(map[string]string{
			"IgnoreUnknown": "1",
		}),
		cni.WithArgs("NERDCTL_CNI_DHCP_HOSTNAME", opts.state.Annotations[labels.Hostname]),
	)
	hsMeta := hostsstore.Meta{
		ID:         opts.state.ID,
		Networks:   make(map[string]*types100.Result, len(opts.cniNames)),
		Hostname:   opts.state.Annotations[labels.Hostname],
		Domainname: opts.state.Annotations[labels.Domainname],
		ExtraHosts: opts.extraHosts,
		Name:       opts.state.Annotations[labels.Name],
	}

	// When containerd gets bounced, containers that were previously running and that are restarted will go again
	// through onCreateRuntime (*unlike* in a normal stop/start flow).
	// As such, a container may very well have an ip already. The bridge plugin would thus refuse to loan a new one
	// and error out, thus making the onCreateRuntime hook fail. In turn, runc (or containerd) will mis-interpret this,
	// and subsequently call onPostStop (although the container will not get deleted), and we will release the name...
	// leading to a bricked system where multiple containers may share the same name.
	// Thus, we do pre-emptively clean things up - error is not checked, as in the majority of cases, that would
	// legitimately error (and that does not matter)
	// See https://github.com/containerd/nerdctl/issues/3355
	_ = opts.cni.Remove(ctx, opts.fullID, "", namespaceOpts...)

	cniRes, err := opts.cni.Setup(ctx, opts.fullID, nsPath, namespaceOpts...)
	if err != nil {
		return fmt.Errorf("failed to call cni.Setup: %w", err)
	}

	defer func() {
		if err != nil {
			log.L.Warn("Container failed starting. Removing allocated network configuration.")
			_ = opts.cni.Remove(ctx, opts.fullID, nsPath, namespaceOpts...)
		}
	}()

	cniResRaw := cniRes.Raw()
	for i, cniName := range opts.cniNames {
		hsMeta.Networks[cniName] = cniResRaw[i]
	}

	b4nnEnabled, b4nnBindEnabled, err := bypass4netnsutil.IsBypass4netnsEnabled(opts.state.Annotations)
	if err != nil {
		return err
	}

	if err := hs.Acquire(hsMeta); err != nil {
		return err
	}

	if rootlessutil.IsRootlessChild() {
		if b4nnEnabled {
			bm, err := bypass4netnsutil.NewBypass4netnsCNIBypassManager(opts.bypassClient, opts.rootlessKitClient, opts.state.Annotations)
			if err != nil {
				return err
			}
			err = bm.StartBypass(ctx, opts.ports, opts.state.ID, opts.state.Annotations[labels.StateDir])
			if err != nil {
				return fmt.Errorf("bypass4netnsd not running? (Hint: run `containerd-rootless-setuptool.sh install-bypass4netnsd`): %w", err)
			}
		}
		if !b4nnBindEnabled && len(opts.ports) > 0 {
			if err := exposePortsRootless(ctx, opts.rootlessKitClient, opts.ports); err != nil {
				return fmt.Errorf("failed to expose ports in rootless mode: %w", err)
			}
		}
	}
	return nil
}

func onCreateRuntime(opts *handlerOpts) error {
	loadAppArmor()

	name := opts.state.Annotations[labels.Name]
	ns := opts.state.Annotations[labels.Namespace]
	namst, err := namestore.New(opts.dataStore, ns)
	if err != nil {
		log.L.WithError(err).Error("failed opening the namestore in onCreateRuntime")
	} else if err := namst.Acquire(name, opts.state.ID); err != nil {
		log.L.WithError(err).Error("failed re-acquiring name - see https://github.com/containerd/nerdctl/issues/2992")
	}

	var netError error
	if opts.cni != nil {
		netError = applyNetworkSettings(opts)
	}

	// Set StartedAt and CreateError
	lf, err := state.New(opts.state.Annotations[labels.StateDir])
	if err != nil {
		return err
	}

	err = lf.Transform(func(lf *state.Store) error {
		lf.StartedAt = time.Now()
		lf.CreateError = netError != nil
		return nil
	})
	if err != nil {
		return err
	}

	return netError
}

func onPostStop(opts *handlerOpts) error {
	lf, err := state.New(opts.state.Annotations[labels.StateDir])
	if err != nil {
		return err
	}

	var shouldExit bool
	err = lf.Transform(func(lf *state.Store) error {
		// See https://github.com/containerd/nerdctl/issues/3357
		// Check if we actually errored during runtimeCreate
		// If that is the case, CreateError is set, and we are in postStop while the container will NOT be deleted (see ticket).
		// Thus, do NOT treat this as a deletion, as the container is still there.
		// Reset CreateError, and return.
		shouldExit = lf.CreateError
		lf.CreateError = false
		return nil
	})
	if err != nil {
		return err
	}
	if shouldExit {
		return nil
	}

	ctx := context.Background()
	ns := opts.state.Annotations[labels.Namespace]
	if opts.cni != nil {
		var err error
		b4nnEnabled, b4nnBindEnabled, err := bypass4netnsutil.IsBypass4netnsEnabled(opts.state.Annotations)
		if err != nil {
			return err
		}
		if rootlessutil.IsRootlessChild() {
			if b4nnEnabled {
				bm, err := bypass4netnsutil.NewBypass4netnsCNIBypassManager(opts.bypassClient, opts.rootlessKitClient, opts.state.Annotations)
				if err != nil {
					return err
				}
				err = bm.StopBypass(ctx, opts.state.ID)
				if err != nil {
					return err
				}
			}
			if !b4nnBindEnabled && len(opts.ports) > 0 {
				if err := unexposePortsRootless(ctx, opts.rootlessKitClient, opts.ports); err != nil {
					return fmt.Errorf("failed to unexpose ports in rootless mode: %w", err)
				}
			}
		}
		portMapOpts, err := getPortMapOpts(opts)
		if err != nil {
			return err
		}
		ipAddressOpts, err := getIPAddressOpts(opts)
		if err != nil {
			return err
		}
		macAddressOpts, err := getMACAddressOpts(opts)
		if err != nil {
			return err
		}
		ip6AddressOpts, err := getIP6AddressOpts(opts)
		if err != nil {
			return err
		}
		var namespaceOpts []cni.NamespaceOpts
		namespaceOpts = append(namespaceOpts, portMapOpts...)
		namespaceOpts = append(namespaceOpts, ipAddressOpts...)
		namespaceOpts = append(namespaceOpts, macAddressOpts...)
		namespaceOpts = append(namespaceOpts, ip6AddressOpts...)
		if err := opts.cni.Remove(ctx, opts.fullID, "", namespaceOpts...); err != nil {
			log.L.WithError(err).Errorf("failed to call cni.Remove")
			return err
		}
		hs, err := hostsstore.New(opts.dataStore, ns)
		if err != nil {
			return err
		}
		if err := hs.Release(opts.state.ID); err != nil {
			return err
		}
	}
	namst, err := namestore.New(opts.dataStore, ns)
	if err != nil {
		return err
	}
	name := opts.state.Annotations[labels.Name]
	// Double-releasing may happen with containers started with --rm, so, ignore NotFound errors
	if err := namst.Release(name, opts.state.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("failed to release container name %s: %w", name, err)
	}
	return nil
}

// writePidFile writes the pid atomically to a file.
// From https://github.com/containerd/containerd/blob/v1.7.0-rc.2/cmd/ctr/commands/commands.go#L265-L282
func writePidFile(path string, pid int) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	tempPath := filepath.Join(filepath.Dir(path), fmt.Sprintf(".%s", filepath.Base(path)))
	f, err := os.OpenFile(tempPath, os.O_RDWR|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0666)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(f, pid)
	f.Close()
	if err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
