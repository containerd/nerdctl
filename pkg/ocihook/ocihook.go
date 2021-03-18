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
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/contrib/apparmor"
	pkgapparmor "github.com/containerd/containerd/pkg/apparmor"
	"github.com/containerd/go-cni"
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/dnsutil/hostsstore"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
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
			return errors.Wrapf(err, "failed to create %q", containerStateDir)
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
		return errors.Errorf("unexpected event %q", event)
	}
}

func newHandlerOpts(state *specs.State, dataStore, cniPath, cniNetconfPath string) (*handlerOpts, error) {
	o := &handlerOpts{
		state:     state,
		dataStore: dataStore,
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
	if len(networks) != 1 {
		return nil, errors.New("currently, number of networks must be 1")
	}

	switch netstr := networks[0]; netstr {
	case "none", "host":
	default:
		e := &netutil.CNIEnv{
			Path:        cniPath,
			NetconfPath: cniNetconfPath,
		}
		ll, err := netutil.ConfigLists(e)
		if err != nil {
			return nil, err
		}
		var netconflist *netutil.NetworkConfigList
		for _, f := range ll {
			if f.Name == netstr {
				netconflist = f
				break
			}
		}
		if netconflist == nil {
			return nil, errors.Errorf("no such network: %q", netstr)
		}
		o.cni, err = gocni.New(gocni.WithPluginDir([]string{cniPath}), gocni.WithConfListBytes(netconflist.Bytes))
		if err != nil {
			return nil, err
		}
		o.cniName = netstr
	}

	if portsJSON := o.state.Annotations[labels.Ports]; portsJSON != "" {
		if err := json.Unmarshal([]byte(portsJSON), &o.ports); err != nil {
			return nil, err
		}
	}
	return o, nil
}

type handlerOpts struct {
	state     *specs.State
	dataStore string
	fullID    string
	rootfs    string
	ports     []gocni.PortMapping
	cni       gocni.CNI // TODO: support multi network
	cniName   string    // TODO: support multi network
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

func getCNINamespaceOpts(opts *handlerOpts) ([]cni.NamespaceOpts, error) {
	if len(opts.ports) > 0 {
		if !rootlessutil.IsRootlessChild() {
			return []cni.NamespaceOpts{cni.WithCapabilityPortMap(opts.ports)}, nil
		}
		// For rootless, we need to modify the hostIP that is not bindable in the child namespace.
		// https: //github.com/containerd/nerdctl/issues/88
		//
		// We must NOT modify opts.ports here, because we use the unmodified opts.ports for
		// interaction with RootlessKit API.
		ports := make([]cni.PortMapping, len(opts.ports))
		for i, p := range opts.ports {
			if hostIP := net.ParseIP(p.HostIP); hostIP != nil {
				// loopback address is always bindable in the child namespace, but other addresses are unlikely.
				if !hostIP.IsLoopback() {
					p.HostIP = "127.0.0.1"
				}
			}
			ports[i] = p
		}
		return []cni.NamespaceOpts{cni.WithCapabilityPortMap(ports)}, nil
	}
	return nil, nil
}

func onCreateRuntime(opts *handlerOpts) error {
	if pkgapparmor.HostSupports() {
		// ensure that the default profile is loaded to the host
		if err := apparmor.LoadDefaultProfile(defaults.AppArmorProfileName); err != nil {
			logrus.WithError(err).Errorf("failed to load AppArmor profile %q", defaults.AppArmorProfileName)
		}
	}
	if opts.cni != nil {
		cniNSOpts, err := getCNINamespaceOpts(opts)
		if err != nil {
			return err
		}
		nsPath, err := getNetNSPath(opts.state)
		if err != nil {
			return err
		}
		ctx := context.Background()
		cniRes, err := opts.cni.Setup(ctx, opts.fullID, nsPath, cniNSOpts...)
		if err != nil {
			return errors.Wrap(err, "failed to call cni.Setup")
		}
		hs, err := hostsstore.NewStore(opts.dataStore)
		if err != nil {
			return err
		}
		hsMeta := hostsstore.Meta{
			Namespace: opts.state.Annotations[labels.Namespace],
			ID:        opts.state.ID,
			Networks: map[string]*cni.CNIResult{
				opts.cniName: cniRes, // TODO: multi-network
			},
			Hostname: opts.state.Annotations[labels.Hostname],
			Name:     opts.state.Annotations[labels.Name],
		}
		if err := hs.Acquire(hsMeta); err != nil {
			return err
		}
		if len(opts.ports) > 0 && rootlessutil.IsRootlessChild() {
			pm, err := rootlessutil.NewRootlessCNIPortManager()
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
			pm, err := rootlessutil.NewRootlessCNIPortManager()
			if err != nil {
				return err
			}
			for _, p := range opts.ports {
				if err := pm.UnexposePort(ctx, p); err != nil {
					return err
				}
			}
		}
		cniNSOpts, err := getCNINamespaceOpts(opts)
		if err != nil {
			return err
		}
		if err := opts.cni.Remove(ctx, opts.fullID, "", cniNSOpts...); err != nil {
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
