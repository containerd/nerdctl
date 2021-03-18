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

package rootlessutil

import (
	"context"
	"net"
	"path/filepath"

	gocni "github.com/containerd/go-cni"
	"github.com/rootless-containers/rootlesskit/pkg/api/client"
	"github.com/rootless-containers/rootlesskit/pkg/port"
)

func NewRootlessCNIPortManager() (*RootlessCNIPortManager, error) {
	stateDir, err := RootlessKitStateDir()
	if err != nil {
		return nil, err
	}
	apiSock := filepath.Join(stateDir, "api.sock")
	client, err := client.New(apiSock)
	if err != nil {
		return nil, err
	}
	pm := &RootlessCNIPortManager{
		Client: client,
	}
	return pm, nil
}

type RootlessCNIPortManager struct {
	client.Client
}

func (rlcpm *RootlessCNIPortManager) ExposePort(ctx context.Context, cpm gocni.PortMapping) error {
	// NOTE: When `nerdctl run -p 8080:80` is being launched, cpm.HostPort is set to 8080 and cpm.ContainerPort is set to 80.
	// We want to forward the port 8080 of the parent namespace into the port 8080 of the child namespace (which is the "host"
	// from the point of view of CNI). So we do NOT set sp.ChildPort to cpm.ContainerPort here.
	sp := port.Spec{
		Proto:      cpm.Protocol,
		ParentIP:   cpm.HostIP,
		ParentPort: int(cpm.HostPort),
		ChildPort:  int(cpm.HostPort), // NOT typo of cpm.ContainerPort
	}
	_, err := rlcpm.Client.PortManager().AddPort(ctx, sp)
	return err
}

func (rlcpm *RootlessCNIPortManager) UnexposePort(ctx context.Context, cpm gocni.PortMapping) error {
	pm := rlcpm.Client.PortManager()
	ports, err := pm.ListPorts(ctx)
	if err != nil {
		return err
	}
	id := -1
	for _, p := range ports {
		sp := p.Spec
		if sp.Proto != cpm.Protocol || sp.ParentPort != int(cpm.HostPort) || sp.ChildPort != int(cpm.HostPort) {
			continue
		}
		spParentIP := net.ParseIP(sp.ParentIP)
		cpmHostIP := net.ParseIP(cpm.HostIP)
		if spParentIP == nil || !spParentIP.Equal(cpmHostIP) {
			continue
		}
		id = p.ID
		break
	}
	if id < 0 {
		// no ID found, return nil for idempotency
		return nil
	}
	return pm.RemovePort(ctx, id)
}
