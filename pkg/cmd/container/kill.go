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

package container

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	gocni "github.com/containerd/go-cni"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/netutil"
	"github.com/containerd/nerdctl/v2/pkg/netutil/nettype"
	"github.com/containerd/nerdctl/v2/pkg/portutil"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/moby/sys/signal"
)

// Kill kills a list of containers
func Kill(ctx context.Context, client *containerd.Client, reqs []string, options types.ContainerKillOptions) error {
	if !strings.HasPrefix(options.KillSignal, "SIG") {
		options.KillSignal = "SIG" + options.KillSignal
	}

	parsedSignal, err := signal.ParseSignal(options.KillSignal)
	if err != nil {
		return err
	}

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			if err := cleanupNetwork(ctx, found.Container, options.GOptions); err != nil {
				return fmt.Errorf("unable to cleanup network for container: %s, %q", found.Req, err)
			}
			if err := killContainer(ctx, found.Container, parsedSignal); err != nil {
				if errdefs.IsNotFound(err) {
					fmt.Fprintf(options.Stderr, "No such container: %s\n", found.Req)
					os.Exit(1)
				}
				return err
			}
			_, err := fmt.Fprintln(options.Stdout, found.Container.ID())
			return err
		},
	}

	return walker.WalkAll(ctx, reqs, true)
}

func killContainer(ctx context.Context, container containerd.Container, signal syscall.Signal) (err error) {
	defer func() {
		if err != nil {
			containerutil.UpdateErrorLabel(ctx, container, err)
		}
	}()
	if err := containerutil.UpdateExplicitlyStoppedLabel(ctx, container, true); err != nil {
		return err
	}
	task, err := container.Task(ctx, cio.Load)
	if err != nil {
		return err
	}

	status, err := task.Status(ctx)
	if err != nil {
		return err
	}

	paused := false

	switch status.Status {
	case containerd.Created, containerd.Stopped:
		return fmt.Errorf("cannot kill container %s: container is not running", container.ID())
	case containerd.Paused, containerd.Pausing:
		paused = true
	default:
	}

	if err := task.Kill(ctx, signal); err != nil {
		return err
	}

	// signal will be sent once resume is finished
	if paused {
		if err := task.Resume(ctx); err != nil {
			log.G(ctx).Warnf("cannot unpause container %s: %s", container.ID(), err)
		}
	}
	return nil
}

// cleanupNetwork removes cni network setup, specifically the forwards
func cleanupNetwork(ctx context.Context, container containerd.Container, globalOpts types.GlobalCommandOptions) error {
	return rootlessutil.WithDetachedNetNSIfAny(func() error {
		// retrieve info to get current active port mappings
		info, err := container.Info(ctx, containerd.WithoutRefreshedMetadata)
		if err != nil {
			return err
		}
		ports, portErr := portutil.ParsePortsLabel(info.Labels)
		if portErr != nil {
			return fmt.Errorf("no oci spec: %q", portErr)
		}
		portMappings := []gocni.NamespaceOpts{
			gocni.WithCapabilityPortMap(ports),
		}

		// retrieve info to get cni instance
		spec, err := container.Spec(ctx)
		if err != nil {
			return err
		}
		networksJSON := spec.Annotations[labels.Networks]
		var networks []string
		if err := json.Unmarshal([]byte(networksJSON), &networks); err != nil {
			return err
		}
		netType, err := nettype.Detect(networks)
		if err != nil {
			return err
		}

		switch netType {
		case nettype.Host, nettype.None, nettype.Container:
			// NOP
		case nettype.CNI:
			e, err := netutil.NewCNIEnv(globalOpts.CNIPath, globalOpts.CNINetConfPath, netutil.WithNamespace(globalOpts.Namespace), netutil.WithDefaultNetwork())
			if err != nil {
				return err
			}
			cniOpts := []gocni.Opt{
				gocni.WithPluginDir([]string{globalOpts.CNIPath}),
			}
			netMap, err := e.NetworkMap()
			if err != nil {
				return err
			}
			for _, netstr := range networks {
				net, ok := netMap[netstr]
				if !ok {
					return fmt.Errorf("no such network: %q", netstr)
				}
				cniOpts = append(cniOpts, gocni.WithConfListBytes(net.Bytes))
			}
			cni, err := gocni.New(cniOpts...)
			if err != nil {
				return err
			}

			var namespaceOpts []gocni.NamespaceOpts
			namespaceOpts = append(namespaceOpts, portMappings...)
			namespace := spec.Annotations[labels.Namespace]
			fullID := namespace + "-" + container.ID()
			if err := cni.Remove(ctx, fullID, "", namespaceOpts...); err != nil {
				log.L.WithError(err).Errorf("failed to call cni.Remove")
				return err
			}
			return nil
		default:
			return fmt.Errorf("unexpected network type %v", netType)
		}
		return nil
	})
}
