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

package bypass4netnsutil

import (
	"context"
	"path/filepath"

	"github.com/containerd/containerd/errdefs"
	gocni "github.com/containerd/go-cni"
	b4nnapi "github.com/rootless-containers/bypass4netns/pkg/api"
	"github.com/rootless-containers/bypass4netns/pkg/api/daemon/client"
)

func NewBypass4netnsCNIBypassManager(client client.Client) (*Bypass4netnsCNIBypassManager, error) {
	if client == nil {
		return nil, errdefs.ErrInvalidArgument
	}
	pm := &Bypass4netnsCNIBypassManager{
		Client: client,
	}
	return pm, nil
}

type Bypass4netnsCNIBypassManager struct {
	client.Client
}

func (b4nnm *Bypass4netnsCNIBypassManager) StartBypass(ctx context.Context, ports []gocni.PortMapping, id, stateDir string) error {
	socketPath, err := GetSocketPathByID(id)
	if err != nil {
		return err
	}
	pidFilePath, err := GetPidFilePathByID(id)
	if err != nil {
		return err
	}
	logFilePath := filepath.Join(stateDir, "bypass4netns.log")

	spec := b4nnapi.BypassSpec{
		ID:          id,
		SocketPath:  socketPath,
		PidFilePath: pidFilePath,
		LogFilePath: logFilePath,
		// TODO: Remove hard-coded subnets
		IgnoreSubnets: []string{"127.0.0.0/8", "10.0.0.0/8"},
	}
	portMap := []b4nnapi.PortSpec{}
	for _, p := range ports {
		portMap = append(portMap, b4nnapi.PortSpec{
			ParentIP:   p.HostIP,
			ParentPort: int(p.HostPort),
			ChildPort:  int(p.ContainerPort),
			Protos:     []string{p.Protocol},
		})
	}
	spec.PortMapping = portMap
	_, err = b4nnm.BypassManager().StartBypass(ctx, spec)
	if err != nil {
		return err
	}

	return nil
}

func (b4nnm *Bypass4netnsCNIBypassManager) StopBypass(ctx context.Context, id string) error {
	err := b4nnm.BypassManager().StopBypass(ctx, id)
	if err != nil {
		return err
	}

	return nil
}
