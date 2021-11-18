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

	"github.com/containerd/containerd/cmd/ctr/commands"
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/containerd/nerdctl/pkg/netutil/nettype"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	types100 "github.com/containernetworking/cni/pkg/types/100"
	dopts "github.com/docker/cli/opts"
	"github.com/opencontainers/runtime-spec/specs-go"

	rlkclient "github.com/rootless-containers/rootlesskit/pkg/api/client"
	"github.com/sirupsen/logrus"
)

func Run(stdin io.Reader, stderr io.Writer, event, dataStore, cniPath, cniNetconfPath string) error {
	if stdin == nil || event == "" || dataStore == "" || cniPath == "" || cniNetconfPath == "" {
		return errors.New("got insufficient args")
	}

	var state specs.State
	if err := json.NewDecoder(stdin).Decode(&state); err != nil {
		return err
	}

	if containerStateDir := state.Annotations[labels.StateDir]; containerStateDir == "" {
		return errors.New("state dir must be set")
	} else {
		if err := os.MkdirAll(containerStateDir, 0700); err != nil {
			return fmt.Errorf("failed to create %q: %w", containerStateDir, err)
		}
		logFilePath := filepath.Join(containerStateDir, "oci-hook."+event+".log")
		logFile, err := os.Create(logFilePath)
		if err != nil {
			return err
		}
		defer logFile.Close()
		logrus.SetOutput(io.MultiWriter(stderr, logFile))
	}

	opts, err := newHandlerOpts(&state, dataStore, cniPath, cniNetconfPath)
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

func newHandlerOpts(state *specs.State, dataStore, cniPath, cniNetconfPath string) (*handlerOpts, error) {
	o := &handlerOpts{
		state:     state,
		dataStore: dataStore,
	}

	extraHostsJSON := o.state.Annotations[labels.ExtraHosts]
	var extraHosts []string
	if err := json.Unmarshal([]byte(extraHostsJSON), &extraHosts); err != nil {
		return nil, err
	}

	//validate and format extraHosts
	ensureExtraHosts := func(extraHosts []string) (map[string]string, error) {
		hosts := make(map[string]string)
		for _, host := range extraHosts {
			hostIP, err := dopts.ValidateExtraHost(host)
			if err != nil {
				return nil, err
			}
			if v := strings.SplitN(hostIP, ":", 2); len(v) == 2 {
				hosts[v[1]] = v[0]
			}
		}
		return hosts, nil
	}

	var err error
	if o.extraHosts, err = ensureExtraHosts(extraHosts); err != nil {
		return nil, err
	}

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
	case nettype.Host, nettype.None:
		// NOP
	case nettype.CNI:
		e := &netutil.CNIEnv{
			Path:        cniPath,
			NetconfPath: cniNetconfPath,
		}
		ll, err := netutil.ConfigLists(e)
		if err != nil {
			return nil, err
		}
		cniOpts := []gocni.Opt{
			gocni.WithPluginDir([]string{cniPath}),
		}
		for _, netstr := range networks {
			var netconflist *netutil.NetworkConfigList
			for _, f := range ll {
				if f.Name == netstr {
					netconflist = f
					break
				}
			}
			if netconflist == nil {
				return nil, fmt.Errorf("no such network: %q", netstr)
			}
			cniOpts = append(cniOpts, gocni.WithConfListBytes(netconflist.Bytes))
			o.cniNames = append(o.cniNames, netstr)
		}
		o.cni, err = gocni.New(cniOpts...)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unexpected network type %v", netType)
	}

	if pidFile := o.state.Annotations[labels.PIDFile]; pidFile != "" {
		if err := commands.WritePidFile(pidFile, state.Pid); err != nil {
			return nil, err
		}
	}

	if portsJSON := o.state.Annotations[labels.Ports]; portsJSON != "" {
		if err := json.Unmarshal([]byte(portsJSON), &o.ports); err != nil {
			return nil, err
		}
	}
	if rootlessutil.IsRootlessChild() {
		o.rootlessKitClient, err = rootlessutil.NewRootlessKitClient()
		if err != nil {
			return nil, err
		}
	}
	return o, nil
}

type handlerOpts struct {
	state             *specs.State
	dataStore         string
	rootfs            string
	ports             []gocni.PortMapping
	cni               gocni.CNI
	cniNames          []string
	fullID            string
	rootlessKitClient rlkclient.Client
	extraHosts        map[string]string // ip:host
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

func getNetNSPath(state *specs.State) (string, error) {
	if state.Pid == 0 {
		return "", errors.New("state.Pid is unset")
	}
	s := fmt.Sprintf("/proc/%d/ns/net", state.Pid)
	if _, err := os.Stat(s); err != nil {
		return "", err
	}
	return s, nil
}

func getPortMapOpts(opts *handlerOpts) ([]gocni.NamespaceOpts, error) {
	if len(opts.ports) > 0 {
		if !rootlessutil.IsRootlessChild() {
			return []gocni.NamespaceOpts{gocni.WithCapabilityPortMap(opts.ports)}, nil
		}
		var (
			childIP                            net.IP
			portDriverDisallowsLoopbackChildIP bool
		)
		info, err := opts.rootlessKitClient.Info(context.TODO())
		if err != nil {
			logrus.WithError(err).Warn("cannot call RootlessKit Info API, make sure you have RootlessKit v0.14.1 or later")
		} else {
			childIP = info.NetworkDriver.ChildIP
			portDriverDisallowsLoopbackChildIP = info.PortDriver.DisallowLoopbackChildIP // true for slirp4netns port driver
		}
		// For rootless, we need to modify the hostIP that is not bindable in the child namespace.
		// https: //github.com/containerd/nerdctl/issues/88
		//
		// We must NOT modify opts.ports here, because we use the unmodified opts.ports for
		// interaction with RootlessKit API.
		ports := make([]gocni.PortMapping, len(opts.ports))
		for i, p := range opts.ports {
			if hostIP := net.ParseIP(p.HostIP); hostIP != nil && !hostIP.IsUnspecified() {
				// loopback address is always bindable in the child namespace, but other addresses are unlikely.
				if !hostIP.IsLoopback() {
					if childIP != nil && childIP.Equal(hostIP) {
						// this is fine
					} else {
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
		return []gocni.NamespaceOpts{gocni.WithCapabilityPortMap(ports)}, nil
	}
	return nil, nil
}

func onCreateRuntime(opts *handlerOpts) error {
	loadAppArmor()

	if opts.cni != nil {
		portMapOpts, err := getPortMapOpts(opts)
		if err != nil {
			return err
		}
		nsPath, err := getNetNSPath(opts.state)
		if err != nil {
			return err
		}
		ctx := context.Background()
		hs, err := hostsstore.NewStore(opts.dataStore)
		if err != nil {
			return err
		}
		hsMeta := hostsstore.Meta{
			Namespace:  opts.state.Annotations[labels.Namespace],
			ID:         opts.state.ID,
			Networks:   make(map[string]*types100.Result, len(opts.cniNames)),
			Hostname:   opts.state.Annotations[labels.Hostname],
			ExtraHosts: opts.extraHosts,
			Name:       opts.state.Annotations[labels.Name],
		}
		cniRes, err := opts.cni.Setup(ctx, opts.fullID, nsPath, portMapOpts...)
		if err != nil {
			return fmt.Errorf("failed to call cni.Setup: %w", err)
		}
		cniResRaw := cniRes.Raw()
		for i, cniName := range opts.cniNames {
			hsMeta.Networks[cniName] = cniResRaw[i]
		}

		if err := hs.Acquire(hsMeta); err != nil {
			return err
		}
		if len(opts.ports) > 0 && rootlessutil.IsRootlessChild() {
			pm, err := rootlessutil.NewRootlessCNIPortManager(opts.rootlessKitClient)
			if err != nil {
				return err
			}
			for _, p := range opts.ports {
				if err := pm.ExposePort(ctx, p); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func onPostStop(opts *handlerOpts) error {
	ctx := context.Background()
	if opts.cni != nil {
		if len(opts.ports) > 0 && rootlessutil.IsRootlessChild() {
			pm, err := rootlessutil.NewRootlessCNIPortManager(opts.rootlessKitClient)
			if err != nil {
				return err
			}
			for _, p := range opts.ports {
				if err := pm.UnexposePort(ctx, p); err != nil {
					return err
				}
			}
		}
		portMapOpts, err := getPortMapOpts(opts)
		if err != nil {
			return err
		}
		if err := opts.cni.Remove(ctx, opts.fullID, "", portMapOpts...); err != nil {
			logrus.WithError(err).Errorf("failed to call cni.Remove")
			return err
		}
		hs, err := hostsstore.NewStore(opts.dataStore)
		if err != nil {
			return err
		}
		ns := opts.state.Annotations[labels.Namespace]
		if err := hs.Release(ns, opts.state.ID); err != nil {
			return err
		}
	}
	return nil
}
