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
	"runtime"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/nerdctl/v2/pkg/ipcutil"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/netutil/nettype"
	"github.com/opencontainers/runtime-spec/specs-go"
)

// ReconfigNetContainer reconfigures the container's network namespace path.
func ReconfigNetContainer(ctx context.Context, c containerd.Container, client *containerd.Client, lab map[string]string) error {
	networksJSON, ok := lab[labels.Networks]
	if !ok {
		return nil
	}
	var networks []string
	if err := json.Unmarshal([]byte(networksJSON), &networks); err != nil {
		return err
	}
	netType, err := nettype.Detect(networks)
	if err != nil {
		return err
	}
	if netType == nettype.Container {
		network := strings.Split(networks[0], ":")
		if len(network) != 2 {
			return fmt.Errorf("invalid network: %s, should be \"container:<id|name>\"", networks[0])
		}
		targetCon, err := client.LoadContainer(ctx, network[1])
		if err != nil {
			return err
		}
		netNSPath, err := ContainerNetNSPath(ctx, targetCon)
		if err != nil {
			return err
		}
		spec, err := c.Spec(ctx)
		if err != nil {
			return err
		}
		err = c.Update(ctx, containerd.UpdateContainerOpts(
			containerd.WithSpec(spec, oci.WithLinuxNamespace(
				specs.LinuxNamespace{
					Type: specs.NetworkNamespace,
					Path: netNSPath,
				}))))
		if err != nil {
			return err
		}
	}
	return nil
}

// ReconfigPIDContainer reconfigures the container's spec options for sharing PID namespace.
func ReconfigPIDContainer(ctx context.Context, c containerd.Container, client *containerd.Client, lab map[string]string) error {
	targetContainerID, ok := lab[labels.PIDContainer]
	if !ok {
		return nil
	}
	if runtime.GOOS != "linux" {
		return errors.New("--pid only supported on linux")
	}
	targetCon, err := client.LoadContainer(ctx, targetContainerID)
	if err != nil {
		return err
	}
	opts, err := GenerateSharingPIDOpts(ctx, targetCon)
	if err != nil {
		return err
	}
	spec, err := c.Spec(ctx)
	if err != nil {
		return err
	}
	err = c.Update(ctx, containerd.UpdateContainerOpts(
		containerd.WithSpec(spec, oci.Compose(opts...)),
	))
	if err != nil {
		return err
	}
	return nil
}

// ReconfigIPCContainer reconfigures the container's spec options for sharing IPC namespace and volumns.
func ReconfigIPCContainer(ctx context.Context, c containerd.Container, client *containerd.Client, lab map[string]string) error {
	ipc, err := ipcutil.DecodeIPCLabel(lab[labels.IPC])
	if err != nil {
		return err
	}
	opts, err := ipcutil.GenerateIPCOpts(ctx, ipc, client)
	if err != nil {
		return err
	}
	spec, err := c.Spec(ctx)
	if err != nil {
		return err
	}
	err = c.Update(ctx, containerd.UpdateContainerOpts(
		containerd.WithSpec(spec, oci.Compose(opts...)),
	))
	if err != nil {
		return err
	}
	return nil
}
